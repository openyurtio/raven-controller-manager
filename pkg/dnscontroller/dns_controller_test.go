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
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func newFakeDnsController(fakeClient client.Interface, nodes []*corev1.Node, pods []*corev1.Pod, services []*corev1.Service) (dc *dnsRecordController) {
	sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 24*time.Hour)
	RegisterInformersForDNS(sharedInformerFactory)

	dc = &dnsRecordController{
		client:              fakeClient,
		syncPeriod:          60,
		informerFactory:     sharedInformerFactory,
		queue:               workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-dns"),
		ravenServiceIPCache: "172.168.0.1",
	}

	nodeInformerFactory := sharedInformerFactory.Core().V1().Nodes()
	nodeInformer := nodeInformerFactory.Informer()
	dc.dnsSharedInformer.nodeLister = nodeInformerFactory.Lister()
	dc.dnsSharedInformer.nodeInformerSynced = nodeInformer.HasSynced

	svcInformerFactory := sharedInformerFactory.Core().V1().Services()
	svcInformer := svcInformerFactory.Informer()
	dc.dnsSharedInformer.svcLister = svcInformerFactory.Lister()
	dc.dnsSharedInformer.svcInformerSynced = svcInformer.HasSynced

	podInformerFactory := sharedInformerFactory.Core().V1().Pods()
	podInformer := podInformerFactory.Informer()
	dc.dnsSharedInformer.podLister = podInformerFactory.Lister()
	dc.dnsSharedInformer.podInformerSynced = podInformer.HasSynced

	podIndexer := podInformer.GetIndexer()
	nodeIndexer := nodeInformer.GetIndexer()
	svcIndexer := svcInformer.GetIndexer()

	for _, po := range pods {
		if podIndexer.Add(po) != nil {
			continue
		}
	}

	for _, node := range nodes {
		if nodeIndexer.Add(node) != nil {
			continue
		}
	}

	for _, svc := range services {
		if svcIndexer.Add(svc) != nil {
			continue
		}
	}

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

func TestDnsRecordController_ManagerRavenDNSRecordConfigMap(t *testing.T) {
	testcases := []struct {
		desc     string
		useCache bool
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   *corev1.ConfigMap
	}{
		{
			desc:     fmt.Sprintf("there is no configmap %v", ravenDNSRecordConfigMapName),
			useCache: false,
			client:   fake.NewSimpleClientset(),
			nodes:    []*corev1.Node{},
			pods:     []*corev1.Pod{},
			services: []*corev1.Service{},
			expect: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenDNSRecordConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					RavenDNSRecordNodeDataKey: "",
				},
			},
		},
		{
			desc:     fmt.Sprintf("there is a configmap %v", ravenDNSRecordConfigMapName),
			useCache: true,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "",
					},
				}),
			nodes:    []*corev1.Node{},
			pods:     []*corev1.Pod{},
			services: []*corev1.Service{},
			expect: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenDNSRecordConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					RavenDNSRecordNodeDataKey: "",
				},
			},
		},
	}
	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			err := fakeDc.manageRavenDNSRecordConfigMap()
			if err != nil {
				t.Logf("failed to create configmap %v/%v", RavenProxyResourceNamespace, ravenDNSRecordConfigMapName)
			}
			get, _ := fakeDc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).
				Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestDnsRecordController_GetRavenProxyInternalService(t *testing.T) {
	testcases := []struct {
		desc     string
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   string
	}{
		{
			desc:   fmt.Sprintf("get service %s", RavenProxyInternalServiceName),
			client: fake.NewSimpleClientset(),
			nodes:  []*corev1.Node{},
			pods:   []*corev1.Pod{},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "172.168.0.1",
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: "172.168.0.1",
		},

		{
			desc:     fmt.Sprintf("failed to get service %s", RavenProxyInternalServiceName),
			client:   fake.NewSimpleClientset(),
			nodes:    []*corev1.Node{},
			pods:     []*corev1.Pod{},
			services: []*corev1.Service{},
			expect:   "",
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			get, err := fakeDc.getRavenInternalServiceIP(false)
			if err != nil {
				t.Logf("failed to get svc %s/%s", RavenProxyResourceNamespace, RavenProxyInternalServiceName)
			}
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestDnsRecordController_UpdateDNSRecordsAsWhole(t *testing.T) {
	getDNSRecord := func(client client.Interface) (string, error) {
		cm, err := client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).
			Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		return cm.Data[RavenDNSRecordNodeDataKey], nil
	}

	testcases := []struct {
		desc     string
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   string
	}{

		{
			desc: "there is no dns records",
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "",
					},
				},
			),
			expect: "",
		},

		{
			desc: "update dns records as whole",
			client: fake.NewSimpleClientset(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenDNSRecordConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					RavenDNSRecordNodeDataKey: "",
				},
			}),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cloud-node-1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "false",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.0.1",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "edge-node-1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.0.2",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "edge-node-2",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.0.3",
							},
						},
					},
				},
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod-1",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "cloud-node-1",
					},
				},

				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod-2",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "edge-node-1",
					},
				},
			},
			services: []*corev1.Service{},
			expect:   "172.168.0.1\tedge-node-1\n192.168.0.1\tcloud-node-1\n192.168.0.3\tedge-node-2",
		},
	}
	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			err := fakeDc.updateDNSRecordsAsWhole()
			if err != nil {
				t.Logf("failed to update dns records as whole")
			}
			get, err := getDNSRecord(fakeDc.client)
			if err != nil {
				t.Logf("failed to get dns records")
			}
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestDnsRecordController_SyncDNSRecordsAsWhole(t *testing.T) {
	getDNSRecord := func(client client.Interface) (string, error) {
		cm, err := client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).
			Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		return cm.Data[RavenDNSRecordNodeDataKey], nil
	}

	testcases := []struct {
		desc     string
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   string
	}{

		{
			desc: "there is no dns records",
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "",
					},
				},
			),
			expect: "",
		},

		{
			desc: "update dns records as whole",
			client: fake.NewSimpleClientset(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenDNSRecordConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					RavenDNSRecordNodeDataKey: "",
				},
			}),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cloud-node-1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "false",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.0.1",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "edge-node-1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.0.2",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "edge-node-2",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.0.3",
							},
						},
					},
				},
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod-1",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "cloud-node-1",
					},
				},

				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod-2",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "edge-node-1",
					},
				},
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "172.168.0.1",
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: "172.168.0.1\tedge-node-1\n192.168.0.1\tcloud-node-1\n192.168.0.3\tedge-node-2",
		},
	}
	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			fakeDc.syncDNSRecordAsWhole()
			get, err := getDNSRecord(fakeDc.client)
			if err != nil {
				t.Logf("failed to get dns records")
			}
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}
