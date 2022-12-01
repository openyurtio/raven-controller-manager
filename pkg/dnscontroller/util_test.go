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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_IsEdgeNode(t *testing.T) {
	testcases := []struct {
		desc   string
		node   *corev1.Node
		expect bool
	}{
		{
			desc: "the node is cloud node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"openyurt.io/is-edge-worker": "false",
					},
				},
			},
			expect: false,
		},
		{
			desc: "the node is edge node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-2",
					Labels: map[string]string{
						"openyurt.io/is-edge-worker": "true",
					},
				},
			},
			expect: true,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			get := isEdgeNode(tt.node)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func Test_FormatDNSRecord(t *testing.T) {
	testcases := []struct {
		desc   string
		ip     string
		host   string
		expect string
	}{
		{
			desc:   "format dns record",
			ip:     "168.172.0.1",
			host:   "node-1",
			expect: "168.172.0.1\tnode-1",
		},
		{
			desc:   "format dns record",
			ip:     "168.172.0.2",
			host:   "node-2",
			expect: "168.172.0.2\tnode-2",
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			get := formatDNSRecord(tt.ip, tt.host)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func Test_GetNodeHostIP(t *testing.T) {
	testcases := []struct {
		desc   string
		node   *corev1.Node
		expect string
	}{
		{
			desc: "the node has no host ip",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"openyurt.io/is-edge-worker": "false",
					},
				},
			},
			expect: "",
		},
		{
			desc: "the type of address is internal ip",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-2",
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
			expect: "192.168.0.1",
		},
		{
			desc: "the type of address is external ip",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-3",
					Labels: map[string]string{
						"openyurt.io/is-edge-worker": "true",
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeExternalIP,
							Address: "172.168.0.1",
						},
					},
				},
			},
			expect: "172.168.0.1",
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			get, _ := getNodeHostIP(tt.node)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func Test_StrToMap(t *testing.T) {
	testcases := []struct {
		desc   string
		dns    string
		expect map[string]string
	}{
		{
			desc: "convert dns record from string to map",
			dns:  "168.172.0.1\tnode-1\n168.172.0.2\tnode-2\n168.172.0.3\tnode-3",
			expect: map[string]string{
				"node-1": "168.172.0.1",
				"node-2": "168.172.0.2",
				"node-3": "168.172.0.3",
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			get := strToMap(&tt.dns)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func Test_MapToStr(t *testing.T) {
	testcases := []struct {
		desc   string
		dns    map[string]string
		expect string
	}{
		{
			desc: "convert dns record from map to string",
			dns: map[string]string{
				"node-1": "168.172.0.1",
				"node-2": "168.172.0.2",
				"node-3": "168.172.0.3",
			},
			expect: "168.172.0.1\tnode-1\n168.172.0.2\tnode-2\n168.172.0.3\tnode-3",
		},
	}

	for _, tt := range testcases {
		t.Run(tt.desc, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.desc)
			get := mapToStr(&tt.dns)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}
