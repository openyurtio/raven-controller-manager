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
	"github.com/openyurtio/raven-controller-manager/pkg/dnscontroller"
	"reflect"
	"sort"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	ravenv1alpha1 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
	ravenClientset "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/clientset/versioned"
	fakeRavenClientset "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/clientset/versioned/fake"
	raveninformers "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/informers/externalversions"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func newFakeServiceController(fakeKubeClient client.Interface, fakeRavenClient ravenClientset.Interface, services []*corev1.Service, configmaps []*corev1.ConfigMap, gateways []*ravenv1alpha1.Gateway) *serviceController {

	sharedInformerFactory := informers.NewSharedInformerFactory(fakeKubeClient, 24*time.Hour)
	RegisterInformersForService(sharedInformerFactory)

	ravenInformerFactory := raveninformers.NewSharedInformerFactory(fakeRavenClient, 24*time.Hour)
	RegisterRavenInformersForService(ravenInformerFactory)

	sc := &serviceController{
		kubeClient:            fakeKubeClient,
		ravenClient:           fakeRavenClient,
		queue:                 workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-service"),
		listenInsecurePort:    InsecurePort,
		listenSecurePort:      SecurePort,
		sharedInformerFactory: sharedInformerFactory,
		ravenInformerFactory:  ravenInformerFactory,
		syncPeriod:            60,
	}

	svcInformerFactory := sharedInformerFactory.Core().V1().Services()
	svcInformer := svcInformerFactory.Informer()
	sc.svcSharedInformer.svcLister = svcInformerFactory.Lister()
	sc.svcSharedInformer.svcInformerSynced = svcInformer.HasSynced

	cmInformerFactory := sharedInformerFactory.Core().V1().ConfigMaps()
	cmInformer := cmInformerFactory.Informer()
	sc.svcSharedInformer.cmLister = cmInformerFactory.Lister()
	sc.svcSharedInformer.cmInformerSynced = cmInformer.HasSynced

	//add event handler for raven informer factory
	gwInformerFactory := ravenInformerFactory.Raven().V1alpha1().Gateways()
	gwInformer := gwInformerFactory.Informer()
	sc.svcSharedInformer.gwLister = gwInformerFactory.Lister()
	sc.svcSharedInformer.gwInformerSynced = gwInformer.HasSynced

	svcIndexer := svcInformer.GetIndexer()
	for _, svc := range services {
		if svcIndexer.Add(svc) != nil {
			continue
		}
	}
	cmIndexer := cmInformer.GetIndexer()
	for _, cm := range configmaps {
		if cmIndexer.Add(cm) != nil {
			continue
		}
	}
	gwIndexer := gwInformer.GetIndexer()
	for _, gw := range gateways {
		if gwIndexer.Add(gw) != nil {
			continue
		}
	}

	// override syncPeriod when the specified value is too small
	if sc.syncPeriod < MinSyncPeriod {
		sc.syncPeriod = MinSyncPeriod
	}

	return sc
}

func getProxyPortService(client client.Interface) (svc *corev1.Service, err error) {
	return client.CoreV1().Services(RavenProxyResourceNamespace).
		Get(context.TODO(), RavenProxyInternalServiceName, metav1.GetOptions{})
}

func getRavenAgent(client client.Interface, name string) (pod *corev1.Pod, err error) {
	return client.CoreV1().Pods(RavenProxyResourceNamespace).
		Get(context.TODO(), name, metav1.GetOptions{})
}

func TestServiceController_GetRavenProxyInternalService(t *testing.T) {
	testcases := []struct {
		desc        string
		kubeClient  client.Interface
		ravenClient ravenClientset.Interface
		gateway     []*ravenv1alpha1.Gateway
		services    []*corev1.Service
		configmap   []*corev1.ConfigMap
		expect      *corev1.Service
	}{
		{
			desc:        fmt.Sprintf("there is no services %s/%s", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
			kubeClient:  fake.NewSimpleClientset(),
			ravenClient: fakeRavenClientset.NewSimpleClientset(),
			services:    []*corev1.Service{},
			configmap:   []*corev1.ConfigMap{},
			gateway:     []*ravenv1alpha1.Gateway{},
			expect:      &corev1.Service{},
		},
		{
			desc:        fmt.Sprintf("there is a services %s/%s", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
			kubeClient:  fake.NewSimpleClientset(),
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
			get, err := fakeDc.getRavenProxyInternalService()
			if err != nil {
				t.Logf("failed to get service %s, %v", RavenProxyInternalServiceName, err)
			}
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestServiceController_UpdateRavenService(t *testing.T) {

	testcases := []struct {
		desc        string
		kubeClient  client.Interface
		ravenClient ravenClientset.Interface
		gateway     []*ravenv1alpha1.Gateway
		services    []*corev1.Service
		configmap   []*corev1.ConfigMap
		ports       map[string]string
		expect      *corev1.Service
	}{
		{
			desc: fmt.Sprintf("add proxy port and update the services %s/%s", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
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
								Name:       "http",
								Port:       10255,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10264),
							},
							{
								Name:       "https",
								Port:       10250,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
						},
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
						Ports: []corev1.ServicePort{
							{
								Name:       "http",
								Port:       10255,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10264),
							},
							{
								Name:       "https",
								Port:       10250,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
						},
					},
				},
			},
			configmap: []*corev1.ConfigMap{},
			gateway:   []*ravenv1alpha1.Gateway{},
			ports: map[string]string{
				"9100": "10263",
				"9445": "10264",
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
							Name:       "http",
							Port:       10255,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10264),
						},
						{
							Name:       "https",
							Port:       10250,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10263),
						},
					},
				},
			},
		},

		{
			desc: fmt.Sprintf("update proxy port and update the services %s/%s", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
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
								Name:       "http",
								Port:       10255,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10264),
							},
							{
								Name:       "https",
								Port:       10250,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
							{
								Name:       "extend-9100",
								Port:       9100,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
						},
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
						Ports: []corev1.ServicePort{
							{
								Name:       "http",
								Port:       10255,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10264),
							},
							{
								Name:       "https",
								Port:       10250,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
							{
								Name:       "extend-9100",
								Port:       9100,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
						},
					},
				},
			},
			configmap: []*corev1.ConfigMap{},
			gateway:   []*ravenv1alpha1.Gateway{},
			ports: map[string]string{
				"9100": "10264",
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
							TargetPort: intstr.FromInt(10264),
						},
						{
							Name:       "http",
							Port:       10255,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10264),
						},
						{
							Name:       "https",
							Port:       10250,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10263),
						},
					},
				},
			},
		},
		{
			desc: fmt.Sprintf("delete proxy port and update the services %s/%s", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
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
								Name:       "http",
								Port:       10255,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10264),
							},
							{
								Name:       "https",
								Port:       10250,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
							{
								Name:       "extend-9100",
								Port:       9100,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
						},
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
						Ports: []corev1.ServicePort{
							{
								Name:       "http",
								Port:       10255,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10264),
							},
							{
								Name:       "https",
								Port:       10250,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
							{
								Name:       "extend-9100",
								Port:       9100,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(10263),
							},
						},
					},
				},
			},
			configmap: []*corev1.ConfigMap{},
			ports:     map[string]string{},
			expect: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      RavenProxyInternalServiceName,
					Namespace: RavenProxyResourceNamespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "172.168.0.1",
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       10255,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10264),
						},
						{
							Name:       "https",
							Port:       10250,
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
			svc, _ := getProxyPortService(fakeDc.kubeClient)
			err := fakeDc.updateRavenService(svc, tt.ports)
			if err != nil {
				t.Logf("failed to update dns records %v", err)
			}
			get, _ := getProxyPortService(tt.kubeClient)

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

func TestServiceController_SyncRavenServiceAsWhole(t *testing.T) {
	testcases := []struct {
		desc        string
		kubeClient  client.Interface
		ravenClient ravenClientset.Interface
		gateway     []*ravenv1alpha1.Gateway
		services    []*corev1.Service
		configmap   []*corev1.ConfigMap
		ports       map[string]string
		expect      *corev1.Service
	}{
		{
			desc: fmt.Sprintf("sync raven service %s/%s with add proxy port", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
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
			gateway: []*ravenv1alpha1.Gateway{},
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

		{
			desc: fmt.Sprintf("sync raven service %s/%s with update proxy port", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
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
						"http-proxy-ports":      "9100",
						"https-proxy-ports":     "9445",
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
			configmap: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenProxyPortConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						"localhost-proxy-ports": "10266, 10267",
						"http-proxy-ports":      "9100",
						"https-proxy-ports":     "9445",
					},
				},
			},
			gateway: []*ravenv1alpha1.Gateway{},
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
							TargetPort: intstr.FromInt(10264),
						},
						{
							Name:       "extend-9445",
							Port:       9445,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(10263),
						},
					},
				},
			},
		},

		{
			desc: fmt.Sprintf("sync raven service %s/%s with delete proxy port", RavenProxyResourceNamespace, RavenProxyInternalServiceName),
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
						"http-proxy-ports":      "",
						"https-proxy-ports":     "",
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
			configmap: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ravenProxyPortConfigMapName,
						Namespace: RavenProxyResourceNamespace,
					},
					Data: map[string]string{
						"localhost-proxy-ports": "10266, 10267",
						"http-proxy-ports":      "",
						"https-proxy-ports":     "",
					},
				},
			},
			gateway: []*ravenv1alpha1.Gateway{},
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
			fakeDc.syncRavenServiceAsWhole()
			get, _ := getProxyPortService(fakeDc.kubeClient)

			sort.Slice(get.Spec.Ports, func(i, j int) bool {
				return get.Spec.Ports[i].Port < get.Spec.Ports[j].Port
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

func TestServiceController_SyncGatewayRavenAgent(t *testing.T) {
	testcases := []struct {
		desc        string
		kubeClient  client.Interface
		ravenClient ravenClientset.Interface
		gateway     []*ravenv1alpha1.Gateway
		services    []*corev1.Service
		configmap   []*corev1.ConfigMap
		expect      map[string]map[string]string
	}{
		{
			desc: "sync gateway raven agent pod",
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
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod-2",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
							ravenServerLabelKey:                 ravenServerLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "cloud-node-1",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "raven-pod-3",
						Namespace: RavenProxyResourceNamespace,
						Labels: map[string]string{
							dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "cloud-node-2",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				}),
			ravenClient: fakeRavenClientset.NewSimpleClientset(),
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
			services:  []*corev1.Service{},
			configmap: []*corev1.ConfigMap{},
			expect: map[string]map[string]string{
				"raven-pod-1": {
					dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
					ravenServerLabelKey:                 ravenServerLabelValue,
				},
				"raven-pod-2": {
					dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
				},
				"raven-pod-3": {
					dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
				},
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeServiceController(tt.kubeClient, tt.ravenClient, tt.services, tt.configmap, tt.gateway)
			fakeDc.syncGateWayRavenAgent()

			pods, err := fakeDc.kubeClient.CoreV1().Pods(RavenProxyResourceNamespace).List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				t.Logf("failed to list raven agent pod")
			}

			for _, get := range pods.Items {
				if !reflect.DeepEqual(get.Labels, tt.expect[get.Name]) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect[get.Name], get.Labels)
			}
		})
	}
}

func TestServiceController_ManagerGatewayRavenAgent(t *testing.T) {
	testcases := []struct {
		desc            string
		kubeClient      client.Interface
		ravenClient     ravenClientset.Interface
		gateway         []*ravenv1alpha1.Gateway
		services        []*corev1.Service
		configmap       []*corev1.ConfigMap
		gatewayNodeName string
		expect          map[string]string
	}{
		{
			desc: "select gateway raven agent pod",
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
				}),
			gatewayNodeName: "gateway-node",
			gateway:         []*ravenv1alpha1.Gateway{},
			services:        []*corev1.Service{},
			configmap:       []*corev1.ConfigMap{},
			expect: map[string]string{
				dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
				ravenServerLabelKey:                 ravenServerLabelValue,
			},
		},

		{
			desc: "update normal raven agent pod",
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
						NodeName: "normal-cloud-node",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				}),
			gatewayNodeName: "gateway-node",
			gateway:         []*ravenv1alpha1.Gateway{},
			services:        []*corev1.Service{},
			configmap:       []*corev1.ConfigMap{},
			expect: map[string]string{
				dnscontroller.RavenAgentPodLabelKey: dnscontroller.RavenAgentPodLabelValue,
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			fakeDc := newFakeServiceController(tt.kubeClient, tt.ravenClient, tt.services, tt.configmap, tt.gateway)
			err := fakeDc.managerGatewayRavenAgent(tt.gatewayNodeName)
			if err != nil {
				t.Logf("failed to get raven agent in node %s", tt.gatewayNodeName)
			}
			get, _ := getRavenAgent(fakeDc.kubeClient, "raven-pod-1")

			if !reflect.DeepEqual(get.Labels, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get.Labels)
			}

			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}
