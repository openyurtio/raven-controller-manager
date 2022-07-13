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

package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ravenv1alpha1 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
)

var (
	nodeReadyStatus = corev1.NodeStatus{
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	nodeNotReadyStatus = corev1.NodeStatus{
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			},
		},
	}
	podReadyStatus = corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	podNotReadyStatus = corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			},
		},
	}
)

func TestGatewayReconciler_electActiveEndpoint(t *testing.T) {
	mockReconciler := &GatewayReconciler{}
	var tt = []struct {
		name                    string
		nodeList                corev1.NodeList
		podList                 corev1.PodList
		gw                      *ravenv1alpha1.Gateway
		expectedActiveEndpoints []*ravenv1alpha1.ActiveEndpoint
	}{
		{
			// The node hosting active endpoint becomes NotReady, and it is the only node in the Gateway,
			// then the active endpoint should be removed.
			name: "lost healthy active endpoint",
			nodeList: corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Status: nodeNotReadyStatus,
					},
				},
			},
			podList: corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: podNotReadyStatus,
					},
				},
			},
			gw: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1alpha1.GatewaySpec{
					Replicas: func(i int) *int { return &i }(1),
					Endpoints: []ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
						},
					},
				},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoints: []*ravenv1alpha1.ActiveEndpoint{
						{
							Endpoint: &ravenv1alpha1.Endpoint{
								NodeName: "node-1",
							},
						},
					},
				},
			},
			expectedActiveEndpoints: []*ravenv1alpha1.ActiveEndpoint{
				{
					Endpoint: &ravenv1alpha1.Endpoint{
						NodeName: "node-1",
					},
					Healthy: false,
				},
			},
		},
		{
			// The node hosting active endpoint becomes NotReady, but there are at least one Ready node,
			// then a new endpoint should be elected active endpoint to replace the old one.
			name: "switch active endpoint",
			nodeList: corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Status: nodeNotReadyStatus,
					}, {
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Status: nodeReadyStatus,
					},
				},
			},
			podList: corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: podNotReadyStatus,
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
						},
						Spec: corev1.PodSpec{
							NodeName: "node-2",
						},
						Status: podReadyStatus,
					},
				},
			},
			gw: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1alpha1.GatewaySpec{
					Replicas: func(i int) *int { return &i }(1),
					Endpoints: []ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
						},
						{
							NodeName: "node-2",
						},
					},
				},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoints: []*ravenv1alpha1.ActiveEndpoint{
						{
							Endpoint: &ravenv1alpha1.Endpoint{
								NodeName: "node-1",
							},
						},
					},
				},
			},
			expectedActiveEndpoints: []*ravenv1alpha1.ActiveEndpoint{
				{
					Endpoint: &ravenv1alpha1.Endpoint{
						NodeName: "node-2",
					},
					Healthy: true,
				},
			},
		},
		{

			name: "elect new active endpoint",
			nodeList: corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Status: nodeNotReadyStatus,
					}, {
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Status: nodeReadyStatus,
					},
				},
			},
			podList: corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: podNotReadyStatus,
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
						},
						Spec: corev1.PodSpec{
							NodeName: "node-2",
						},
						Status: podReadyStatus,
					},
				},
			},
			gw: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1alpha1.GatewaySpec{
					Replicas: func(i int) *int { return &i }(1),
					Endpoints: []ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
						},
						{
							NodeName: "node-2",
						},
					},
				},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoints: []*ravenv1alpha1.ActiveEndpoint{
						{
							Endpoint: &ravenv1alpha1.Endpoint{
								NodeName: "node-1",
							},
						},
					},
				},
			},
			expectedActiveEndpoints: []*ravenv1alpha1.ActiveEndpoint{
				{
					Endpoint: &ravenv1alpha1.Endpoint{
						NodeName: "node-2",
					},
					Healthy: true,
				},
			},
		},
		{
			name: "no available active endpoint",
			nodeList: corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Status: nodeNotReadyStatus,
					}, {
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Status: nodeNotReadyStatus,
					},
				},
			},
			podList: corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: podNotReadyStatus,
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
						},
						Spec: corev1.PodSpec{
							NodeName: "node-2",
						},
						Status: podNotReadyStatus,
					},
				},
			},
			gw: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1alpha1.GatewaySpec{
					Replicas: func(i int) *int { return &i }(1),
					Endpoints: []ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
						},
						{
							NodeName: "node-2",
						},
					},
				},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoints: []*ravenv1alpha1.ActiveEndpoint{},
				},
			},
			expectedActiveEndpoints: []*ravenv1alpha1.ActiveEndpoint{
				{
					Endpoint: &ravenv1alpha1.Endpoint{
						NodeName: "node-1",
					},
					Healthy: false,
				},
			},
		},
		{
			// The node hosting the active endpoint is still ready, do not change it.
			name: "don't switch active endpoint",
			nodeList: corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Status: nodeNotReadyStatus,
					}, {
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Status: nodeReadyStatus,
					},
				},
			},
			podList: corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: podNotReadyStatus,
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
						},
						Spec: corev1.PodSpec{
							NodeName: "node-2",
						},
						Status: podReadyStatus,
					},
				},
			},
			gw: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1alpha1.GatewaySpec{
					Replicas: func(i int) *int { return &i }(1),
					Endpoints: []ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
						},
						{
							NodeName: "node-2",
						},
					},
				},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoints: []*ravenv1alpha1.ActiveEndpoint{
						{
							Endpoint: &ravenv1alpha1.Endpoint{
								NodeName: "node-2",
							},
						},
					},
				},
			},
			expectedActiveEndpoints: []*ravenv1alpha1.ActiveEndpoint{
				{
					Endpoint: &ravenv1alpha1.Endpoint{
						NodeName: "node-2",
					},
					Healthy: true,
				},
			},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ep, _ := mockReconciler.electActiveEndpoints(v.nodeList, v.podList, v.gw)
			a.Equal(v.expectedActiveEndpoints, ep)
		})
	}

}
