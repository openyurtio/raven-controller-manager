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
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_GetProxyPortsMap(t *testing.T) {
	testcases := []struct {
		desc      string
		configmap *corev1.ConfigMap
		expect    map[string]string
	}{
		{
			desc: fmt.Sprintf("get proxy port map from configmap %v/%v", RavenProxyResourceNamespace, ravenProxyPortConfigMapName),
			configmap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenProxyPortConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					"localhost-proxy-ports": "10250, 10255,10266, 10267",
					"http-proxy-ports":      "9445",
					"https-proxy-ports":     "9100",
				},
			},
			expect: map[string]string{
				"9445": InsecurePort,
				"9100": SecurePort,
			},
		},

		{
			desc: fmt.Sprintf("delete proxy port 10255 and 10250 map from configmap %v/%v", RavenProxyResourceNamespace, ravenProxyPortConfigMapName),
			configmap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ravenProxyPortConfigMapName,
					Namespace: RavenProxyResourceNamespace,
				},
				Data: map[string]string{
					"localhost-proxy-ports": "10266, 10267",
					"http-proxy-ports":      "9445, 10255",
					"https-proxy-ports":     "9100, 10250",
				},
			},
			expect: map[string]string{
				"9445": InsecurePort,
				"9100": SecurePort,
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			get := getProxyPortsMap(tt.configmap, InsecurePort, SecurePort)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func Test_UpdateServicePortsRecord(t *testing.T) {
	testcases := []struct {
		desc           string
		oldServicePort map[string]corev1.ServicePort
		newServicePort map[string]corev1.ServicePort
		expect         struct {
			ports     []corev1.ServicePort
			isChanged bool
		}
	}{
		{
			desc:           "add service port",
			oldServicePort: map[string]corev1.ServicePort{},
			newServicePort: map[string]corev1.ServicePort{
				"TCP:9100": {
					Name:       "extend-9100",
					Port:       9100,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(10250),
				},
			},
			expect: struct {
				ports     []corev1.ServicePort
				isChanged bool
			}{
				ports: []corev1.ServicePort{
					{
						Name:       "extend-9100",
						Port:       9100,
						Protocol:   corev1.ProtocolTCP,
						TargetPort: intstr.FromInt(10250),
					},
				},
				isChanged: true,
			},
		},

		{
			desc: "update service port",
			oldServicePort: map[string]corev1.ServicePort{
				"TCP:9100": {
					Name:       "extend-9100",
					Port:       9100,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(10250),
				},
			},
			newServicePort: map[string]corev1.ServicePort{
				"TCP:9100": {
					Name:       "extend-9100",
					Port:       9100,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(10255),
				},
			},
			expect: struct {
				ports     []corev1.ServicePort
				isChanged bool
			}{
				ports: []corev1.ServicePort{
					{
						Name:       "extend-9100",
						Port:       9100,
						Protocol:   corev1.ProtocolTCP,
						TargetPort: intstr.FromInt(10255),
					},
				},
				isChanged: true,
			},
		},

		{
			desc: "update service port",
			oldServicePort: map[string]corev1.ServicePort{
				"TCP:9100": {
					Name:       "extend-9100",
					Port:       9100,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(10250),
				},
			},
			newServicePort: map[string]corev1.ServicePort{
				"TCP:9100": {
					Name:       "extend-9100",
					Port:       9100,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(10250),
				},
			},
			expect: struct {
				ports     []corev1.ServicePort
				isChanged bool
			}{
				ports: []corev1.ServicePort{
					{
						Name:       "extend-9100",
						Port:       9100,
						Protocol:   corev1.ProtocolTCP,
						TargetPort: intstr.FromInt(10250),
					},
				},
				isChanged: false,
			},
		},

		{
			desc: "delete service port",
			oldServicePort: map[string]corev1.ServicePort{
				"TCP:9100": {
					Name:       "extend-9100",
					Port:       9100,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(10250),
				},
			},
			newServicePort: map[string]corev1.ServicePort{},
			expect: struct {
				ports     []corev1.ServicePort
				isChanged bool
			}{
				ports:     []corev1.ServicePort{},
				isChanged: true,
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			isChanged, get := updateServicePortsRecord(tt.oldServicePort, tt.newServicePort)
			if !reflect.DeepEqual(get, tt.expect.ports) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect.ports, get)
			}

			if !reflect.DeepEqual(isChanged, tt.expect.isChanged) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect.isChanged, isChanged)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}
