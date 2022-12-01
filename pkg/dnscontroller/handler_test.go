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
	"sort"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	tunnelIP = "172.168.0.1"
)

func TestDnsRecordController_AddNode(t *testing.T) {

	testcases := []struct {
		desc     string
		useCache bool
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   []string
	}{
		{
			desc:     "add a cloud node",
			useCache: false,
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
						Name: "cloud-node-2",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "false",
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
			},
			pods: []*corev1.Pod{},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.0.1", "cloud-node-1"),
				formatDNSRecord("192.168.0.2", "cloud-node-2"),
			},
		},
		{
			desc:     "add a edge node",
			useCache: false,
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
			nodes: []*corev1.Node{
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
								Address: "192.168.1.1",
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
								Address: "192.168.1.2",
							},
						},
					},
				},
			},
			pods: []*corev1.Pod{},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.1.1", "edge-node-1"),
				formatDNSRecord("192.168.1.2", "edge-node-2"),
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			for _, node := range tt.nodes {
				fakeDc.addNode(node)
			}
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			cm, err := fakeDc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
			if err != nil {
				t.Errorf("failed to get configmap, %v", err)
			}
			get := strings.Split(cm.Data[RavenDNSRecordNodeDataKey], "\n")

			sort.Strings(tt.expect)
			sort.Strings(get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Errorf("expect to get records %v, but got %v", tt.expect, get)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestDnsRecordController_UpdateNode(t *testing.T) {
	tunnelIP := "172.168.0.1"
	testcases := []struct {
		desc     string
		useCache bool
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   []string
	}{
		{
			desc:     "update cloud node to edge node without raven agent",
			useCache: false,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "192.168.0.1\tnode-1",
					},
				},
			),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
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
						Name: "node-1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.1",
							},
						},
					},
				},
			},
			pods: []*corev1.Pod{},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.1.1", "node-1"),
			},
		},
		{
			desc:     "update edge node to cloud node without raven agent",
			useCache: false,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "192.168.1.1\tnode-1",
					},
				},
			),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.1",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
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
			},
			pods: []*corev1.Pod{},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.0.1", "node-1"),
			},
		},
		{
			desc:     "update cloud node to edge node with raven agent",
			useCache: false,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "192.168.0.1\tnode-1",
					},
				},
			),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
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
						Name: "node-1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.1",
							},
						},
					},
				},
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node-1",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord(tunnelIP, "node-1"),
			},
		},
		{
			desc:     "update edge node to cloud node raven agent",
			useCache: false,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "172.168.0.1\tnode-1",
					},
				},
			),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.1",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
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
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node-1",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.0.1", "node-1"),
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			fakeDc.updateNode(tt.nodes[0], tt.nodes[1])
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			cm, err := fakeDc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
			if err != nil {
				t.Errorf("failed to get configmap, %v", err)
			}
			get := strings.Split(cm.Data[RavenDNSRecordNodeDataKey], "\n")

			sort.Strings(tt.expect)
			sort.Strings(get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Errorf("expect to get records %v, but got %v", tt.expect, get)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestDnsRecordController_DeleteNode(t *testing.T) {
	tunnelIP := "172.168.0.1"
	testcases := []struct {
		desc     string
		useCache bool
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   []string
	}{
		{
			desc:     "delete a cloud node",
			useCache: false,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "192.168.0.1\tcloud-node-1\n192.168.0.2\tcloud-node-2",
					},
				},
			),
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
						Name: "cloud-node-2",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "false",
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
			},
			pods: []*corev1.Pod{},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.0.1", "cloud-node-1"),
			},
		},
		{
			desc:     "delete a edge node",
			useCache: false,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "192.168.1.1\tedge-node-1\n192.168.1.2\tedge-node-2",
					},
				},
			),
			nodes: []*corev1.Node{
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
								Address: "192.168.1.1",
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
								Address: "192.168.1.2",
							},
						},
					},
				},
			},
			pods: []*corev1.Pod{},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.1.1", "edge-node-1"),
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			fakeDc.deleteNode(tt.nodes[1])
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			cm, err := fakeDc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
			if err != nil {
				t.Errorf("failed to get configmap, %v", err)
			}
			get := strings.Split(cm.Data[RavenDNSRecordNodeDataKey], "\n")

			sort.Strings(tt.expect)
			sort.Strings(get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Errorf("expect to get records %v, but got %v", tt.expect, get)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestDnsRecordController_AddService(t *testing.T) {
	testcases := []struct {
		desc     string
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   []string
	}{
		{
			desc: fmt.Sprintf("add service %s/%s", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
			client: fake.NewSimpleClientset(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenDNSRecordConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					RavenDNSRecordNodeDataKey: "192.168.0.1\tcloud-node-1\n192.168.0.2\tedge-node-1\n192.168.0.3\tedge-node-2",
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
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},

				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod-3",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "edge-node-2",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
			expect: []string{
				formatDNSRecord("192.168.0.1", "cloud-node-1"),
				formatDNSRecord(tunnelIP, "edge-node-1"),
				formatDNSRecord(tunnelIP, "edge-node-2"),
			},
		},
	}
	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			fakeDc.addService(tt.services[0])
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			cm, err := fakeDc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
			if err != nil {
				t.Errorf("failed to get configmap, %v", err)
			}
			get := strings.Split(cm.Data[RavenDNSRecordNodeDataKey], "\n")

			sort.Strings(tt.expect)
			sort.Strings(get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Errorf("expect to get records %v, but got %v", tt.expect, get)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestDnsRecordController_UpdateService(t *testing.T) {
	testcases := []struct {
		desc     string
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   []string
	}{
		{
			desc: fmt.Sprintf("add service %s/%s", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
			client: fake.NewSimpleClientset(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenDNSRecordConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					RavenDNSRecordNodeDataKey: "192.168.0.1\tcloud-node-1\n172.168.0.1\tedge-node-1\n172.168.0.1\tedge-node-2",
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
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},

				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod-3",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "edge-node-2",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "172.168.1.1",
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.0.1", "cloud-node-1"),
				formatDNSRecord("172.168.1.1", "edge-node-1"),
				formatDNSRecord("172.168.1.1", "edge-node-2"),
			},
		},
	}
	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			fakeDc.updateService(tt.services[0], tt.services[1])
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			cm, err := fakeDc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
			if err != nil {
				t.Errorf("failed to get configmap, %v", err)
			}
			get := strings.Split(cm.Data[RavenDNSRecordNodeDataKey], "\n")

			sort.Strings(tt.expect)
			sort.Strings(get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Errorf("expect to get records %v, but got %v", tt.expect, get)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestDnsRecordController_AddPod(t *testing.T) {

	testcases := []struct {
		desc     string
		useCache bool
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   []string
	}{
		{
			desc:     "add a raven agent in cloud node",
			useCache: false,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "192.168.0.1\tcloud-node",
					},
				},
			),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cloud-node",
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
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "cloud-node",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.0.1", "cloud-node"),
			},
		},
		{
			desc:     "add a raven pod in edge node",
			useCache: false,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "192.168.1.1\tedge-node",
					},
				},
			),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "edge-node",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.1",
							},
						},
					},
				},
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "edge-node",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord(tunnelIP, "edge-node"),
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			for _, pod := range tt.pods {
				fakeDc.addPod(pod)
			}
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			cm, err := fakeDc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
			if err != nil {
				t.Errorf("failed to get configmap, %v", err)
			}
			get := strings.Split(cm.Data[RavenDNSRecordNodeDataKey], "\n")

			sort.Strings(tt.expect)
			sort.Strings(get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Errorf("expect to get records %v, but got %v", tt.expect, get)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestDnsRecordController_DeletePod(t *testing.T) {

	testcases := []struct {
		desc     string
		useCache bool
		client   client.Interface
		nodes    []*corev1.Node
		pods     []*corev1.Pod
		services []*corev1.Service
		expect   []string
	}{
		{
			desc:     "delete a raven agent in cloud node",
			useCache: false,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "192.168.0.1\tcloud-node",
					},
				},
			),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cloud-node",
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
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "cloud-node",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.0.1", "cloud-node"),
			},
		},
		{
			desc:     "add a raven pod in edge node",
			useCache: false,
			client: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenDNSRecordConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						RavenDNSRecordNodeDataKey: "172.168.0.1\tedge-node",
					},
				},
			),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "edge-node",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.1",
							},
						},
					},
				},
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							RavenAgentPodLabelKey: RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "edge-node",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
						ClusterIP: tunnelIP,
						Ports:     []corev1.ServicePort{},
					},
				},
			},
			expect: []string{
				formatDNSRecord("192.168.1.1", "edge-node"),
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeDnsController(tt.client, tt.nodes, tt.pods, tt.services)
			fakeDc.deletePod(tt.pods[0])
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			cm, err := fakeDc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
			if err != nil {
				t.Errorf("failed to get configmap, %v", err)
			}
			get := strings.Split(cm.Data[RavenDNSRecordNodeDataKey], "\n")

			sort.Strings(tt.expect)
			sort.Strings(get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Errorf("expect to get records %v, but got %v", tt.expect, get)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}
