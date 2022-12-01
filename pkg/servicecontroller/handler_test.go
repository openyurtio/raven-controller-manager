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
	"reflect"
	"sort"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/raven-controller-manager/pkg/dnscontroller"
	ravenv1alpha1 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
	ravenClientset "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/clientset/versioned"
	fakeRavenClientset "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/clientset/versioned/fake"
)

func TestServiceController_AddService(t *testing.T) {
	testcases := []struct {
		desc        string
		kubeClient  client.Interface
		ravenClient ravenClientset.Interface
		gateway     []*ravenv1alpha1.Gateway
		services    []*corev1.Service
		configmap   []*corev1.ConfigMap
		addService  *corev1.Service
		expect      *corev1.Service
	}{
		{
			desc: fmt.Sprintf("add services %s/%s", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
			kubeClient: fake.NewSimpleClientset(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "172.168.0.1",
						Ports:     []corev1.ServicePort{},
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenProxyPortConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						"localhost-proxy-ports": "10266, 10267",
						"http-proxy-ports":      "9445",
						"https-proxy-ports":     "9100",
					},
				},
			),
			ravenClient: fakeRavenClientset.NewSimpleClientset(),
			services:    []*corev1.Service{},
			configmap: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenProxyPortConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						"localhost-proxy-ports": "10266, 10267",
						"http-proxy-ports":      "9445",
						"https-proxy-ports":     "9100",
					},
				},
			},
			addService: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      RavenProxyInternalServiceName,
					Namespace: RavenProxyResourceNamespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "172.168.0.1",
					Ports:     []corev1.ServicePort{},
				},
			},
			expect: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      RavenProxyInternalServiceName,
					Namespace: RavenProxyResourceNamespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "172.168.0.1",
					Ports: []corev1.ServicePort{
						{
							Name:       "extend-9100",
							Port:       9100,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10263),
						},
						{
							Name:       "extend-9445",
							Port:       9445,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10264),
						},
					},
				},
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeServiceController(tt.kubeClient, tt.ravenClient, tt.services, tt.configmap, tt.gateway)
			fakeDc.addService(tt.addService)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			get, _ := getProxyPortService(fakeDc.kubeClient)

			sort.Slice(get.Spec.Ports, func(i, j int) bool {
				return get.Spec.Ports[i].Port >= get.Spec.Ports[j].Port
			})

			sort.Slice(tt.expect.Spec.Ports, func(i, j int) bool {
				return tt.expect.Spec.Ports[i].Port >= tt.expect.Spec.Ports[j].Port
			})

			if !reflect.DeepEqual(get.Spec.Ports, tt.expect.Spec.Ports) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect.Spec.Ports, get.Spec.Ports)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

		})
	}
}

func TestServiceController_AddConfigMap(t *testing.T) {
	testcases := []struct {
		desc         string
		kubeClient   client.Interface
		ravenClient  ravenClientset.Interface
		gateway      []*ravenv1alpha1.Gateway
		services     []*corev1.Service
		configmap    []*corev1.ConfigMap
		addConfigMap *corev1.ConfigMap
		expect       *corev1.Service
	}{
		{
			desc: fmt.Sprintf("add configmap %s/%v", RavenProxyResourceNamespace, ravenProxyPortConfigMapName),
			kubeClient: fake.NewSimpleClientset(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "172.168.0.1",
						Ports:     []corev1.ServicePort{},
					},
				},
			),
			ravenClient: fakeRavenClientset.NewSimpleClientset(),
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
			configmap: []*corev1.ConfigMap{},
			gateway:   []*ravenv1alpha1.Gateway{},
			addConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenProxyPortConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					"localhost-proxy-ports": "10266, 10267",
					"http-proxy-ports":      "9445",
					"https-proxy-ports":     "9100",
				},
			},
			expect: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      RavenProxyInternalServiceName,
					Namespace: RavenProxyResourceNamespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "172.168.0.1",
					Ports: []corev1.ServicePort{
						{
							Name:       "extend-9100",
							Port:       9100,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10263),
						},
						{
							Name:       "extend-9445",
							Port:       9445,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10264),
						},
					},
				},
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeServiceController(tt.kubeClient, tt.ravenClient, tt.services, tt.configmap, tt.gateway)
			fakeDc.addConfigMap(tt.addConfigMap)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			get, _ := getProxyPortService(fakeDc.kubeClient)

			sort.Slice(get.Spec.Ports, func(i, j int) bool {
				return get.Spec.Ports[i].Port >= get.Spec.Ports[j].Port
			})

			sort.Slice(tt.expect.Spec.Ports, func(i, j int) bool {
				return tt.expect.Spec.Ports[i].Port >= tt.expect.Spec.Ports[j].Port
			})

			if !reflect.DeepEqual(get.Spec.Ports, tt.expect.Spec.Ports) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect.Spec.Ports, get.Spec.Ports)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

		})
	}
}

func TestServiceController_UpdateConfigMap(t *testing.T) {
	testcases := []struct {
		desc         string
		kubeClient   client.Interface
		ravenClient  ravenClientset.Interface
		gateway      []*ravenv1alpha1.Gateway
		services     []*corev1.Service
		configmap    []*corev1.ConfigMap
		oldConfigMap *corev1.ConfigMap
		newConfigMap *corev1.ConfigMap
		expect       *corev1.Service
	}{
		{
			desc: fmt.Sprintf("add configmap %s/%v", RavenProxyResourceNamespace, ravenProxyPortConfigMapName),
			kubeClient: fake.NewSimpleClientset(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "172.168.0.1",
						Ports:     []corev1.ServicePort{},
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenProxyPortConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						"localhost-proxy-ports": "10266, 10267",
						"http-proxy-ports":      "9445",
						"https-proxy-ports":     "9100",
					},
				},
			),
			ravenClient: fakeRavenClientset.NewSimpleClientset(),
			gateway:     []*ravenv1alpha1.Gateway{},
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
			configmap: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenProxyPortConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						"localhost-proxy-ports": "10266, 10267",
						"http-proxy-ports":      "9445",
						"https-proxy-ports":     "9100",
					},
				},
			},
			oldConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenProxyPortConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					"localhost-proxy-ports": "10266, 10267",
					"http-proxy-ports":      "9445",
					"https-proxy-ports":     "9100",
				},
			},
			newConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenProxyPortConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					"localhost-proxy-ports": "10266, 10267",
					"http-proxy-ports":      "9445",
					"https-proxy-ports":     "9100, 80",
				},
			},
			expect: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      RavenProxyInternalServiceName,
					Namespace: RavenProxyResourceNamespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "172.168.0.1",
					Ports: []corev1.ServicePort{
						{
							Name:       "extend-9100",
							Port:       9100,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10263),
						},
						{
							Name:       "extend-9445",
							Port:       9445,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10264),
						},
						{
							Name:       "extend-80",
							Port:       80,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10263),
						},
					},
				},
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeServiceController(tt.kubeClient, tt.ravenClient, tt.services, tt.configmap, tt.gateway)
			fakeDc.updateConfigMap(tt.oldConfigMap, tt.newConfigMap)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			get, _ := getProxyPortService(fakeDc.kubeClient)

			sort.Slice(get.Spec.Ports, func(i, j int) bool {
				return get.Spec.Ports[i].Port >= get.Spec.Ports[j].Port
			})

			sort.Slice(tt.expect.Spec.Ports, func(i, j int) bool {
				return tt.expect.Spec.Ports[i].Port >= tt.expect.Spec.Ports[j].Port
			})

			if !reflect.DeepEqual(get.Spec.Ports, tt.expect.Spec.Ports) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect.Spec.Ports, get.Spec.Ports)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

		})
	}
}

func TestServiceController_DeleteConfigMap(t *testing.T) {
	testcases := []struct {
		desc            string
		kubeClient      client.Interface
		ravenClient     ravenClientset.Interface
		gateway         []*ravenv1alpha1.Gateway
		services        []*corev1.Service
		configmap       []*corev1.ConfigMap
		deleteConfigMap *corev1.ConfigMap
		expect          *corev1.Service
	}{
		{
			desc: fmt.Sprintf("delete configmap %s/%v", RavenProxyResourceNamespace, ravenProxyPortConfigMapName),
			kubeClient: fake.NewSimpleClientset(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "172.168.0.1",
						Ports: []corev1.ServicePort{
							{
								Name:       "extend-9100",
								Port:       9100,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
							{
								Name:       "extend-9445",
								Port:       9445,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10264),
							},
							{
								Name:       "extend-80",
								Port:       80,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
						},
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenProxyPortConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						"localhost-proxy-ports": "10266, 10267",
						"http-proxy-ports":      "9445",
						"https-proxy-ports":     "9100",
					},
				},
			),
			ravenClient: fakeRavenClientset.NewSimpleClientset(),
			gateway:     []*ravenv1alpha1.Gateway{},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      RavenProxyInternalServiceName,
						Namespace: RavenProxyResourceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "172.168.0.1",
						Ports: []corev1.ServicePort{
							{
								Name:       "extend-9100",
								Port:       9100,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
							{
								Name:       "extend-9445",
								Port:       9445,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10264),
							},
							{
								Name:       "extend-80",
								Port:       80,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
						},
					},
				},
			},
			configmap: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenProxyPortConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						"localhost-proxy-ports": "10266, 10267",
						"http-proxy-ports":      "9445",
						"https-proxy-ports":     "9100",
					},
				},
			},
			deleteConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenProxyPortConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					"localhost-proxy-ports": "10266, 10267",
					"http-proxy-ports":      "9445",
					"https-proxy-ports":     "9100",
				},
			},
			expect: &corev1.Service{
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
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeServiceController(tt.kubeClient, tt.ravenClient, tt.services, tt.configmap, tt.gateway)
			fakeDc.deleteConfigMap(tt.deleteConfigMap)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			get, _ := getProxyPortService(fakeDc.kubeClient)

			sort.Slice(get.Spec.Ports, func(i, j int) bool {
				return get.Spec.Ports[i].Port > -get.Spec.Ports[j].Port
			})

			sort.Slice(tt.expect.Spec.Ports, func(i, j int) bool {
				return tt.expect.Spec.Ports[i].Port < tt.expect.Spec.Ports[j].Port
			})

			if !reflect.DeepEqual(get.Spec.Ports, tt.expect.Spec.Ports) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect.Spec.Ports, get.Spec.Ports)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

		})
	}
}

func TestServiceController_AddGateWay(t *testing.T) {
	testcases := []struct {
		desc        string
		kubeClient  client.Interface
		ravenClient ravenClientset.Interface
		gateway     []*ravenv1alpha1.Gateway
		services    []*corev1.Service
		configmap   []*corev1.ConfigMap
		addGateway  *ravenv1alpha1.Gateway
		expect      map[string]string
	}{
		{
			desc: "add gateway",
			kubeClient: fake.NewSimpleClientset(
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod-1",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "gateway-node",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			),
			ravenClient: fakeRavenClientset.NewSimpleClientset(),
			services:    []*corev1.Service{},
			configmap:   []*corev1.ConfigMap{},
			gateway: []*ravenv1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cloud-nodepool",
						Labels: map[string]string{
							ravenServerLabelKey: ravenServerLabelValue,
						},
					},
					Spec: ravenv1alpha1.GatewaySpec{},
					Status: ravenv1alpha1.GatewayStatus{
						ActiveEndpoint: &ravenv1alpha1.Endpoint{
							NodeName: "gateway-node",
						},
					},
				},
			},
			addGateway: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-nodepool",
					Labels: map[string]string{
						ravenServerLabelKey: ravenServerLabelValue,
					},
				},
				Spec: ravenv1alpha1.GatewaySpec{},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoint: &ravenv1alpha1.Endpoint{
						NodeName: "gateway-node",
					},
				},
			},
			expect: map[string]string{
				dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
				ravenServerLabelKey:                 ravenServerLabelValue,
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeServiceController(tt.kubeClient, tt.ravenClient, tt.services, tt.configmap, tt.gateway)
			fakeDc.addGateWay(tt.addGateway)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			podList, _ := fakeDc.kubeClient.CoreV1().Pods(RavenProxyResourceNamespace).List(context.TODO(), metav1.ListOptions{})
			get := podList.Items[0]
			if !reflect.DeepEqual(get.Labels, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get.Labels)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

		})
	}
}

func TestServiceController_DeleteGateWay(t *testing.T) {
	testcases := []struct {
		desc          string
		kubeClient    client.Interface
		ravenClient   ravenClientset.Interface
		gateway       []*ravenv1alpha1.Gateway
		services      []*corev1.Service
		configmap     []*corev1.ConfigMap
		deleteGateway *ravenv1alpha1.Gateway
		expect        map[string]string
	}{
		{
			desc: "delete gateway",
			kubeClient: fake.NewSimpleClientset(
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod-1",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
							ravenServerLabelKey:                 ravenServerLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "gateway-node",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			),
			ravenClient: fakeRavenClientset.NewSimpleClientset(),
			services:    []*corev1.Service{},
			configmap:   []*corev1.ConfigMap{},
			gateway: []*ravenv1alpha1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cloud-nodepool",
						Labels: map[string]string{
							ravenServerLabelKey: ravenServerLabelValue,
						},
					},
					Spec: ravenv1alpha1.GatewaySpec{},
					Status: ravenv1alpha1.GatewayStatus{
						ActiveEndpoint: &ravenv1alpha1.Endpoint{
							NodeName: "gateway-node",
						},
					},
				},
			},
			deleteGateway: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-nodepool",
					Labels: map[string]string{
						ravenServerLabelKey: ravenServerLabelValue,
					},
				},
				Spec: ravenv1alpha1.GatewaySpec{},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoint: &ravenv1alpha1.Endpoint{
						NodeName: "gateway-node",
					},
				},
			},
			expect: map[string]string{
				dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeServiceController(tt.kubeClient, tt.ravenClient, tt.services, tt.configmap, tt.gateway)
			fakeDc.deleteGateWay(tt.deleteGateway)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if fakeDc.queue.Len() == 0 {
						fakeDc.queue.ShutDown()
					}
				}
			}()
			fakeDc.worker()
			podList, _ := fakeDc.kubeClient.CoreV1().Pods(RavenProxyResourceNamespace).List(context.TODO(), metav1.ListOptions{})
			get := podList.Items[0]
			if !reflect.DeepEqual(get.Labels, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get.Labels)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

		})
	}
}
