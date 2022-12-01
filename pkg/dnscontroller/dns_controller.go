/*
Copyright 2022 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dnscontroller

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// DNSRecordController interface defines the method for manage
// the dns records of node from cloud to edge
type DNSRecordController interface {
	Run(stopCh <-chan struct{})
}

type dnsRecordController struct {
	lock            sync.Mutex
	client          clientset.Interface
	informerFactory informers.SharedInformerFactory
	queue           workqueue.RateLimitingInterface

	dnsSharedInformer   dnsInformer
	syncPeriod          int
	ravenServiceIPCache string

	getPodsAssignedToNode func(nodeName string) ([]*corev1.Pod, error)
}

type dnsInformer struct {
	nodeLister corelisters.NodeLister
	svcLister  corelisters.ServiceLister
	podLister  corelisters.PodLister

	nodeInformerSynced cache.InformerSynced
	svcInformerSynced  cache.InformerSynced
	podInformerSynced  cache.InformerSynced
}

// NewCoreDNSRecordController create a CoreDNSRecordController that synchronizes node dns records with CoreDNS configuration
func NewCoreDNSRecordController(client clientset.Interface, syncPeriod int) DNSRecordController {
	sharedInformerFactory := informers.NewSharedInformerFactory(client, 24*time.Hour)
	RegisterInformersForDNS(sharedInformerFactory)

	dc := &dnsRecordController{
		client:          client,
		syncPeriod:      syncPeriod,
		informerFactory: sharedInformerFactory,
		queue:           workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-dns"),
	}

	nodeInformerFactory := sharedInformerFactory.Core().V1().Nodes()
	nodeInformer := nodeInformerFactory.Informer()
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    dc.addNode,
		UpdateFunc: dc.updateNode,
		DeleteFunc: dc.deleteNode,
	})
	dc.dnsSharedInformer.nodeLister = nodeInformerFactory.Lister()
	dc.dnsSharedInformer.nodeInformerSynced = nodeInformer.HasSynced

	svcInformerFactory := sharedInformerFactory.Core().V1().Services()
	svcInformer := svcInformerFactory.Informer()
	svcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    dc.addService,
		UpdateFunc: dc.updateService,
		DeleteFunc: dc.deleteService,
	})
	dc.dnsSharedInformer.svcLister = svcInformerFactory.Lister()
	dc.dnsSharedInformer.svcInformerSynced = svcInformer.HasSynced

	podInformerFactory := sharedInformerFactory.Core().V1().Pods()
	podInformer := podInformerFactory.Informer()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    dc.addPod,
		UpdateFunc: dc.updatePod,
		DeleteFunc: dc.deletePod,
	})
	dc.dnsSharedInformer.podLister = podInformerFactory.Lister()
	dc.dnsSharedInformer.podInformerSynced = podInformer.HasSynced

	podIndexer := podInformer.GetIndexer()
	dc.getPodsAssignedToNode = func(nodeName string) ([]*corev1.Pod, error) {
		objs, err := podIndexer.ByIndex(NodeNameKeyIndex, nodeName)
		if err != nil {
			return nil, err
		}
		pods := make([]*corev1.Pod, 0, len(objs))
		for _, obj := range objs {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				continue
			}
			pods = append(pods, pod)
		}
		return pods, nil
	}

	// override syncPeriod when the specified value is too small
	if dc.syncPeriod < MinSyncPeriod {
		dc.syncPeriod = MinSyncPeriod
	}

	return dc
}

func (dc *dnsRecordController) Run(stopCh <-chan struct{}) {
	electionChecker := leaderelection.NewLeaderHealthzAdaptor(time.Second * 30)
	id, err := os.Hostname()
	if err != nil {
		klog.Fatalf("failed to get hostname, %v", err)
	}
	resourceLock, err := resourcelock.New("leases", metav1.NamespaceSystem, "raven-dns-controller",
		dc.client.CoreV1(),
		dc.client.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: id + "_" + string(uuid.NewUUID()),
		})
	if err != nil {
		klog.Fatalf("error creating raven-dns-controller resource lock, %v", err)
	}
	leaderelection.RunOrDie(context.Background(), leaderelection.LeaderElectionConfig{
		Lock:          resourceLock,
		LeaseDuration: metav1.Duration{Duration: time.Second * time.Duration(15)}.Duration,
		RenewDeadline: metav1.Duration{Duration: time.Second * time.Duration(10)}.Duration,
		RetryPeriod:   metav1.Duration{Duration: time.Second * time.Duration(2)}.Duration,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				klog.V(3).Infoln("raven dns controller start to running")
				dc.run(stopCh)
			},
			OnStoppedLeading: func() {
				klog.Fatalf("leader election has stopped")
			},
		},
		WatchDog: electionChecker,
		Name:     "raven-dns-controller",
	})
	panic("unreachable")
}

func (dc *dnsRecordController) run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer dc.queue.ShutDown()

	klog.Infof("starting raven dns record controller")
	defer klog.Infof("shutting down raven dns record controller")

	dc.informerFactory.Start(stopCh)

	if !cache.WaitForNamedCacheSync("raven-dns-controller", stopCh,
		dc.dnsSharedInformer.nodeInformerSynced,
		dc.dnsSharedInformer.svcInformerSynced,
		dc.dnsSharedInformer.podInformerSynced) {
		return
	}

	if err := dc.manageRavenDNSRecordConfigMap(); err != nil {
		klog.Errorf("failed to manage dns record ConfigMap %v/%v, %v",
			RavenProxyResourceNamespace, ravenDNSRecordConfigMapName, err)
		return
	}

	go wait.Until(dc.worker, time.Second, stopCh)

	go wait.Until(dc.syncDNSRecordAsWhole, time.Duration(dc.syncPeriod)*time.Second, stopCh)

	<-stopCh
}

func (dc *dnsRecordController) enqueue(eventType EventType, obj interface{}) {
	ele := &Event{
		Type: eventType,
		Obj:  obj,
	}
	dc.queue.Add(ele)
}

func (dc *dnsRecordController) worker() {
	for dc.processNextWorkItem() {
	}
}

func (dc *dnsRecordController) processNextWorkItem() bool {
	event, quit := dc.queue.Get()
	if quit {
		return false
	}
	defer dc.queue.Done(event)

	err := dc.dispatch(event.(*Event))
	dc.handleErr(err, event)
	return true
}

func (dc *dnsRecordController) dispatch(event *Event) error {
	switch event.Type {
	case NodeAdd:
		return dc.onAddNode(event.Obj.(*corev1.Node))
	case NodeUpdate:
		return dc.onUpdateNode(event.Obj.(*corev1.Node))
	case NodeDelete:
		return dc.onDeleteNode(event.Obj.(*corev1.Node))
	case ServiceAdd:
		return dc.onAddService()
	case ServiceUpdate:
		return dc.onUpdateService()
	case PodAdd:
		return dc.onAddPod(event.Obj.(*corev1.Pod))
	case PodUpdate:
		return dc.onUpdatePod(event.Obj.(*corev1.Pod))
	case PodDelete:
		return dc.onDeletePod(event.Obj.(*corev1.Pod))
	default:
		return nil
	}
}

func (dc *dnsRecordController) handleErr(err error, event interface{}) {
	if err == nil {
		dc.queue.Forget(event)
		return
	}

	if dc.queue.NumRequeues(event) < MaxRetries {
		klog.Infof("error syncing event %v: %v", event, err)
		dc.queue.AddRateLimited(event)
		return
	}
	utilruntime.HandleError(err)
	klog.Infof("dropping event %q out of the queue: %v", event, err)
	dc.queue.Forget(event)
}

func (dc *dnsRecordController) manageRavenDNSRecordConfigMap() error {
	_, err := dc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).
		Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ravenDNSRecordConfigMapName,
				Namespace: RavenProxyResourceNamespace,
			},
			Data: map[string]string{
				RavenDNSRecordNodeDataKey: "",
			},
		}
		_, err = dc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).
			Create(context.TODO(), cm, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create ConfigMap %v/%v, %w",
				RavenProxyResourceNamespace, ravenDNSRecordConfigMapName, err)
		}
	}
	return err
}

func (dc *dnsRecordController) syncDNSRecordAsWhole() {
	klog.V(2).Info("start sync dns record as whole")

	dc.lock.Lock()
	defer dc.lock.Unlock()
	_, err := dc.getRavenInternalServiceIP(false)
	if err != nil {
		klog.Errorf("failed to sync dns record as whole, %v", err)
		return
	}

	err = dc.updateDNSRecordsAsWhole()
	if err != nil {
		klog.Errorf("failed to sync dns record as whole, %v", err)
		return
	}
}

func (dc *dnsRecordController) getRavenInternalServiceIP(useCache bool) (string, error) {

	if useCache && len(dc.ravenServiceIPCache) != 0 {
		return dc.ravenServiceIPCache, nil
	}
	svc, err := dc.dnsSharedInformer.svcLister.Services(RavenProxyResourceNamespace).Get(RavenProxyInternalServiceName)
	if err != nil {
		return "", fmt.Errorf("failed to get %v/%v service, %w",
			RavenProxyResourceNamespace, RavenProxyInternalServiceName, err)
	}

	if len(svc.Spec.ClusterIP) == 0 {
		return "", fmt.Errorf("unable find ClusterIP from %s/%s service, %w",
			RavenProxyResourceNamespace, RavenProxyInternalServiceName, err)
	}

	dc.ravenServiceIPCache = svc.Spec.ClusterIP

	return dc.ravenServiceIPCache, nil
}

func (dc *dnsRecordController) updateDNSRecordsAsWhole() (err error) {
	// list all raven agent pod
	pods, err := dc.dnsSharedInformer.podLister.List(labels.Everything())

	if err != nil {
		klog.Errorf("failed to sync dns record as whole, %v", err)
	}
	tables := make(map[string]struct{})
	for _, pod := range pods {
		if len(pod.Spec.NodeName) == 0 {
			continue
		}
		tables[pod.Spec.NodeName] = struct{}{}
	}

	//list all node
	nodes, err := dc.dnsSharedInformer.nodeLister.List(labels.Everything())

	if err != nil {
		klog.Errorf("failed to sync dns record as whole, %v", err)
		return
	}

	//format dns record
	records := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ip, err := getNodeHostIP(node)
		if err != nil {
			klog.Errorf("failed to parse node address for %v, %v", node.Name, err)
			continue
		}

		if _, ok := tables[node.Name]; ok && isEdgeNode(node) {
			ip, err = dc.getRavenInternalServiceIP(true)
			if err != nil {
				klog.Errorf("failed to get service ClusterIP address for %v, %v", node.Name, err)
				continue
			}
		}
		records = append(records, formatDNSRecord(ip, node.Name))
	}
	sort.Strings(records)

	cm, err := dc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).
		Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get configmap %v/%v for sync dns record as whole, %v",
			RavenProxyResourceNamespace, ravenDNSRecordConfigMapName, err)
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[RavenDNSRecordNodeDataKey] = strings.Join(records, "\n")
	if _, err := dc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).
		Update(context.Background(), cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update configmap %v/%v, %w",
			RavenProxyResourceNamespace, ravenDNSRecordConfigMapName, err)
	}
	return nil
}
