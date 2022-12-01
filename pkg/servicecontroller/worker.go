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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/openyurtio/raven-controller-manager/pkg/dnscontroller"
	ravenv1alpha1 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
)

func (sc *serviceController) onServiceAdd(svc *corev1.Service) error {
	cm, err := sc.svcSharedInformer.cmLister.ConfigMaps(RavenProxyResourceNamespace).Get(ravenProxyPortConfigMapName)
	if err != nil {
		klog.Errorf("failed to get configmap %v/%v, when sync raven server internal service: %w", RavenProxyResourceNamespace, ravenProxyPortConfigMapName, err)
		return err
	}

	_, err = sc.kubeClient.CoreV1().Services(RavenProxyResourceNamespace).Get(context.TODO(), RavenProxyInternalServiceName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("failed to get service %s/%s, %w", RavenProxyResourceNamespace, RavenProxyInternalServiceName, err)
		return err
	}

	portsMap := getProxyPortsMap(cm, sc.listenInsecurePort, sc.listenSecurePort)
	if err = sc.updateRavenService(svc, portsMap); err != nil {
		klog.Errorf("failed to sync raven server internal service, %w", err)
	}
	return err
}

func (sc *serviceController) onConfigMapAdd(cm *corev1.ConfigMap) error {
	svc, err := sc.getRavenProxyInternalService()
	if err != nil {
		klog.Errorf("failed to sync raven server internal service, %w", err)
		return err
	}

	portsMap := getProxyPortsMap(cm, sc.listenInsecurePort, sc.listenSecurePort)
	if err = sc.updateRavenService(svc, portsMap); err != nil {
		klog.Errorf("failed to sync raven server internal service, %w", err)
	}
	return err
}

func (sc *serviceController) onConfigMapUpdate(cm *corev1.ConfigMap) error {
	svc, err := sc.getRavenProxyInternalService()
	if err != nil {
		klog.Errorf("failed to sync raven server internal service, %w", err)
		return err
	}

	portsMap := getProxyPortsMap(cm, sc.listenInsecurePort, sc.listenSecurePort)
	if err = sc.updateRavenService(svc, portsMap); err != nil {
		klog.Errorf("failed to sync raven server internal service, %w", err)
	}
	return err
}

func (sc *serviceController) onConfigMapDelete(cm *corev1.ConfigMap) error {
	svc, err := sc.getRavenProxyInternalService()
	if err != nil {
		klog.Errorf("failed to sync raven server internal service, %w", err)
		return err
	}

	portsMap := make(map[string]string, 0)
	if err = sc.updateRavenService(svc, portsMap); err != nil {
		klog.Errorf("failed to sync raven server internal service, %w", err)
	}
	return err
}

func (sc *serviceController) onGatewayAdd(gw *ravenv1alpha1.Gateway) error {
	return sc.managerRavenServer(gw.Status.ActiveEndpoint.NodeName, true)
}

func (sc *serviceController) onGatewayUpdate(gw *ravenv1alpha1.Gateway) error {
	return sc.managerRavenServer(gw.Status.ActiveEndpoint.NodeName, true)
}

func (sc *serviceController) onGatewayDelete(gw *ravenv1alpha1.Gateway) error {
	return sc.managerRavenServer(gw.Status.ActiveEndpoint.NodeName, false)
}

// note: fake clientset cannot simulate the logic of extract object values to match field selectors is not available from within the fake client
func (sc *serviceController) managerRavenServer(nodeName string, isGateway bool) error {
	options := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", dnscontroller.RavenAgentPodLabelKey, dnscontroller.RavenAgentPodLabelValue),
		FieldSelector: fmt.Sprintf("%s=%s", "spec.nodeName", nodeName),
	}
	ravenAgents, err := sc.kubeClient.CoreV1().Pods(RavenProxyResourceNamespace).List(context.TODO(), options)
	if err != nil || len(ravenAgents.Items) < 1 {
		klog.Errorf("failed to get gateway raven agent, %w", err)
		return err
	}
	var gatewayRavenAgent *corev1.Pod = nil
	for _, ravenAgent := range ravenAgents.Items {
		if ravenAgent.Status.Phase == corev1.PodRunning {
			gatewayRavenAgent = &ravenAgent
			break
		}
	}

	if gatewayRavenAgent == nil {
		return fmt.Errorf("failed get running raven agent in gateway node %s", nodeName)
	}

	if isGateway {
		gatewayRavenAgent.Labels[ravenServerLabelKey] = ravenServerLabelValue
	} else {
		_, ok := gatewayRavenAgent.Labels[ravenServerLabelKey]
		if ok {
			delete(gatewayRavenAgent.Labels, ravenServerLabelKey)
		}
	}

	_, err = sc.kubeClient.CoreV1().Pods(RavenProxyResourceNamespace).Update(context.TODO(), gatewayRavenAgent, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("failed to update gateway raven agent, %w", err)
	}
	return nil
}
