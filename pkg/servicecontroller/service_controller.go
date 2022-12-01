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

package servicecontroller

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
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

	"github.com/openyurtio/raven-controller-manager/pkg/dnscontroller"
	ravenv1alpha1 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
	ravenclientset "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/clientset/versioned"
	raveninformers "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/informers/externalversions"
	gatewaylisters "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/listers/raven/v1alpha1"
)

type ServiceController interface {
	Run(stopCh <-chan struct{})
}

type serviceController struct {
	kubeClient            clientset.Interface
	ravenClient           ravenclientset.Interface
	sharedInformerFactory informers.SharedInformerFactory
	ravenInformerFactory  raveninformers.SharedInformerFactory
	queue                 workqueue.RateLimitingInterface

	svcSharedInformer  serviceInformer
	syncPeriod         int
	listenInsecurePort string
	listenSecurePort   string
}

type serviceInformer struct {
	svcLister corelisters.ServiceLister
	cmLister  corelisters.ConfigMapLister
	gwLister  gatewaylisters.GatewayLister

	svcInformerSynced cache.InformerSynced
	cmInformerSynced  cache.InformerSynced
	gwInformerSynced  cache.InformerSynced
}

func NewServiceController(kubeClient clientset.Interface, ravenClient ravenclientset.Interface, syncPeriod int) ServiceController {

	// register k8s cluster resource informer
	sharedInformerFactory := informers.NewSharedInformerFactory(kubeClient, 24*time.Hour)
	RegisterInformersForService(sharedInformerFactory)

	//register openyurt cluster resource informer
	ravenInformerFactory := raveninformers.NewSharedInformerFactory(ravenClient, 24*time.Hour)
	RegisterRavenInformersForService(ravenInformerFactory)

	sc := &serviceController{
		kubeClient:            kubeClient,
		ravenClient:           ravenClient,
		syncPeriod:            syncPeriod,
		listenInsecurePort:    InsecurePort,
		listenSecurePort:      SecurePort,
		sharedInformerFactory: sharedInformerFactory,
		ravenInformerFactory:  ravenInformerFactory,
		queue:                 workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-service"),
	}

	//add event handler for shared informer factory
	svcInformerFactory := sharedInformerFactory.Core().V1().Services()
	svcInformer := svcInformerFactory.Informer()
	svcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: sc.addService,
	})
	sc.svcSharedInformer.svcLister = svcInformerFactory.Lister()
	sc.svcSharedInformer.svcInformerSynced = svcInformer.HasSynced

	cmInformerFactory := sharedInformerFactory.Core().V1().ConfigMaps()
	cmInformer := cmInformerFactory.Informer()
	cmInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    sc.addConfigMap,
		UpdateFunc: sc.updateConfigMap,
		DeleteFunc: sc.deleteConfigMap,
	})
	sc.svcSharedInformer.cmLister = cmInformerFactory.Lister()
	sc.svcSharedInformer.cmInformerSynced = cmInformer.HasSynced

	//add event handler for raven informer factory
	gwInformerFactory := ravenInformerFactory.Raven().V1alpha1().Gateways()
	gwInformer := gwInformerFactory.Informer()
	gwInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    sc.addGateWay,
		UpdateFunc: sc.updateGateWay,
		DeleteFunc: sc.deleteGateWay,
	})
	sc.svcSharedInformer.gwLister = gwInformerFactory.Lister()
	sc.svcSharedInformer.gwInformerSynced = gwInformer.HasSynced

	// override syncPeriod when the specified value is too small
	if sc.syncPeriod < MinSyncPeriod {
		sc.syncPeriod = MinSyncPeriod
	}

	return sc
}

func (sc *serviceController) Run(stopCh <-chan struct{}) {
	electionChecker := leaderelection.NewLeaderHealthzAdaptor(time.Second * 30)
	id, err := os.Hostname()
	if err != nil {
		klog.Fatalf("failed to get hostname, %v", err)
	}
	resourceLock, err := resourcelock.New("leases", metav1.NamespaceSystem, "raven-svc-controller",
		sc.kubeClient.CoreV1(),
		sc.kubeClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: id + "_" + string(uuid.NewUUID()),
		})
	if err != nil {
		klog.Fatalf("error creating raven-svc-controller resource lock, %v", err)
	}
	leaderelection.RunOrDie(context.Background(), leaderelection.LeaderElectionConfig{
		Lock:          resourceLock,
		LeaseDuration: metav1.Duration{Duration: time.Second * time.Duration(15)}.Duration,
		RenewDeadline: metav1.Duration{Duration: time.Second * time.Duration(10)}.Duration,
		RetryPeriod:   metav1.Duration{Duration: time.Second * time.Duration(2)}.Duration,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				klog.V(3).Infoln("raven service controller start to running")
				sc.run(stopCh)
			},
			OnStoppedLeading: func() {
				klog.Fatalf("leader election has stopped")
			},
		},
		WatchDog: electionChecker,
		Name:     "raven-svc-controller",
	})
	panic("unreachable")
}

func (sc *serviceController) run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer sc.queue.ShutDown()

	klog.Infof("starting raven service controller")
	defer klog.Infof("shutting down raven service controller")

	sc.sharedInformerFactory.Start(stopCh)
	sc.ravenInformerFactory.Start(stopCh)
	if !cache.WaitForNamedCacheSync("raven-service-controller",
		stopCh,
		sc.svcSharedInformer.svcInformerSynced,
		sc.svcSharedInformer.cmInformerSynced,
		sc.svcSharedInformer.gwInformerSynced,
	) {
		return
	}

	go wait.Until(sc.worker, time.Second, stopCh)

	go wait.Until(sc.syncRavenServiceAsWhole, time.Duration(sc.syncPeriod)*time.Second, stopCh)

	go wait.Until(sc.syncGateWayRavenAgent, time.Duration(sc.syncPeriod)*time.Second, stopCh)

	<-stopCh
}

func (sc *serviceController) enqueue(eventType EventType, obj interface{}) {
	ele := &Event{
		Type: eventType,
		Obj:  obj,
	}
	sc.queue.Add(ele)
}

func (sc *serviceController) worker() {
	for sc.processNextWorkItem() {
	}
}

func (sc *serviceController) processNextWorkItem() bool {
	event, quit := sc.queue.Get()
	if quit {
		return false
	}
	defer sc.queue.Done(event)

	err := sc.dispatch(event.(*Event))
	sc.handleErr(err, event)
	return true
}

func (sc *serviceController) dispatch(event *Event) error {
	switch event.Type {
	case ServiceAdd:
		return sc.onServiceAdd(event.Obj.(*corev1.Service))
	case ConfigMapAdd:
		return sc.onConfigMapAdd(event.Obj.(*corev1.ConfigMap))
	case ConfigMapUpdate:
		return sc.onConfigMapUpdate(event.Obj.(*corev1.ConfigMap))
	case ConfigMapDelete:
		return sc.onConfigMapDelete(event.Obj.(*corev1.ConfigMap))
	case GatewayAdd:
		return sc.onGatewayAdd(event.Obj.(*ravenv1alpha1.Gateway))
	case GatewayUpdate:
		return sc.onGatewayUpdate(event.Obj.(*ravenv1alpha1.Gateway))
	case GatewayDelete:
		return sc.onGatewayDelete(event.Obj.(*ravenv1alpha1.Gateway))
	default:
		return nil
	}
}

func (sc *serviceController) handleErr(err error, event interface{}) {
	if err == nil {
		sc.queue.Forget(event)
		return
	}

	if sc.queue.NumRequeues(event) < MaxRetries {
		klog.Infof("error syncing event %v: %v", event, err)
		sc.queue.AddRateLimited(event)
		return
	}
	utilruntime.HandleError(err)
	klog.Infof("dropping event %q out of the queue: %v", event, err)
	sc.queue.Forget(event)
}

func (sc *serviceController) syncRavenServiceAsWhole() {
	klog.V(2).Info("start sync raven server service as whole")
	svc, err := sc.getRavenProxyInternalService()
	if err != nil {
		klog.Errorf("failed to sync raven server internal service, %w", err)
		return
	}

	cm, err := sc.svcSharedInformer.cmLister.ConfigMaps(RavenProxyResourceNamespace).Get(ravenProxyPortConfigMapName)
	if err != nil {
		klog.Errorf("failed to get configmap %v/%v, when sync raven server internal service: %w", RavenProxyResourceNamespace, ravenProxyPortConfigMapName, err)
		return
	}

	portsMap := getProxyPortsMap(cm, sc.listenInsecurePort, sc.listenSecurePort)
	if err = sc.updateRavenService(svc, portsMap); err != nil {
		klog.Errorf("failed to sync raven server internal service, %w", err)
	}
}

func (sc *serviceController) getRavenProxyInternalService() (*corev1.Service, error) {
	svc, err := sc.svcSharedInformer.svcLister.Services(RavenProxyResourceNamespace).Get(RavenProxyInternalServiceName)
	if err != nil {
		return &corev1.Service{}, err
	}
	return svc, nil
}

func (sc *serviceController) updateRavenService(svc *corev1.Service, portsMap map[string]string) error {
	//record current service ports information
	ravenServicePortMap := make(map[string]corev1.ServicePort, len(svc.Spec.Ports))
	for _, svcPort := range svc.Spec.Ports {
		portKey := fmt.Sprintf("%s:%d", svcPort.Protocol, svcPort.Port)
		ravenServicePortMap[portKey] = svcPort
	}

	//record new service ports information
	ServicePortMap := make(map[string]corev1.ServicePort, len(portsMap))
	for port, dstPort := range portsMap {
		portName := fmt.Sprintf("%s-%s", RavenExtendedPort, port)
		portInt, err := strconv.ParseInt(port, 10, 32)
		if err != nil {
			klog.Errorf("failed to parse port, %v", err)
			continue
		}
		targetPortInt, err := strconv.Atoi(dstPort)
		if err != nil {
			klog.Errorf("failed to parse target port, %v", err)
			continue
		}
		portKey := fmt.Sprintf("%s:%s", corev1.ProtocolTCP, port)
		ServicePortMap[portKey] = corev1.ServicePort{
			Name:       portName,
			Port:       int32(portInt),
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(targetPortInt),
		}
	}

	isChanged, svcPortsRecord := updateServicePortsRecord(ravenServicePortMap, ServicePortMap)
	if isChanged {
		svc.Spec.Ports = svcPortsRecord
		_, err := sc.kubeClient.CoreV1().Services(RavenProxyResourceNamespace).
			Update(context.Background(), svc, metav1.UpdateOptions{})
		if err != nil {
			klog.Errorf("failed to sync and update raven server service, %v", err)
			return err
		}
	}

	return nil
}

func (sc *serviceController) syncGateWayRavenAgent() {
	klog.V(2).Info("start sync gateway raven agent as whole")
	cloudGateway, err := sc.svcSharedInformer.gwLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("failed to sync gateway raven agent as whole, %w", err)
		return
	}
	if len(cloudGateway) < 1 || cloudGateway[0].Status.ActiveEndpoint == nil {
		klog.Errorln("there are no gateway in cloud nodepool")
		return
	}

	err = sc.managerGatewayRavenAgent(cloudGateway[0].Status.ActiveEndpoint.NodeName)
	if err != nil {
		klog.Errorf("failed to manager gateway raven agent in node %s, %s", cloudGateway[0].Status.ActiveEndpoint.NodeName, err)
	}
}

func (sc *serviceController) managerGatewayRavenAgent(gateway string) error {
	options := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", dnscontroller.RavenAgentPodLabelKey, dnscontroller.RavenAgentPodLabelValue),
	}
	ravenAgents, err := sc.kubeClient.CoreV1().Pods(RavenProxyResourceNamespace).List(context.TODO(), options)
	if err != nil {
		klog.Errorf("failed to get gateway raven agent, %w", err)
		return err
	}

	for _, ravenAgent := range ravenAgents.Items {
		if len(ravenAgent.Spec.NodeName) < 1 {
			continue
		}

		isChanged := false
		if ravenAgent.Spec.NodeName == gateway {
			ravenAgent.Labels[ravenServerLabelKey] = ravenServerLabelValue
			isChanged = true
		} else {
			_, ok := ravenAgent.Labels[ravenServerLabelKey]
			if ok {
				delete(ravenAgent.Labels, ravenServerLabelKey)
				isChanged = true
			}
		}

		if isChanged {
			_, err = sc.kubeClient.CoreV1().Pods(RavenProxyResourceNamespace).Update(context.TODO(), &ravenAgent, metav1.UpdateOptions{})
			if err != nil {
				klog.Errorf("failed to update gateway raven agent, %w", err)
			}
		}
	}
	return nil
}
