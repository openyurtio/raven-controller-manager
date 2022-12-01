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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func (dc *dnsRecordController) onAddNode(node *corev1.Node) error {

	dnsRecords, err := dc.getCurrentRavenDNSRecord()
	if err != nil {
		return err
	}

	ip, err := getNodeHostIP(node)
	if err != nil {
		klog.Fatalf("failed to parse node address for %v, %v", node.Name, err)
		return err
	}

	dnsRecordsMap := strToMap(&dnsRecords)
	dnsRecordsMap[node.Name] = ip
	dnsRecordsStr := mapToStr(&dnsRecordsMap)

	err = dc.updateCurrentRavenDNSRecord(dnsRecordsStr)
	if err != nil {
		return err
	}
	return nil
}

func (dc *dnsRecordController) onUpdateNode(node *corev1.Node) error {
	dnsRecords, err := dc.getCurrentRavenDNSRecord()
	if err != nil {
		return err
	}

	newIP, err := dc.getUpdatedNodeIP(node)
	if err != nil {
		klog.Fatalf("failed to get node address for %v, %v", node.Name, err)
		return err
	}

	dnsRecordsMap := strToMap(&dnsRecords)
	if oldIP, ok := dnsRecordsMap[node.Name]; ok && oldIP != newIP {
		dnsRecordsMap[node.Name] = newIP
		dnsRecordsStr := mapToStr(&dnsRecordsMap)

		err = dc.updateCurrentRavenDNSRecord(dnsRecordsStr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dc *dnsRecordController) getUpdatedNodeIP(node *corev1.Node) (string, error) {

	ip, err := getNodeHostIP(node)
	if err != nil {
		klog.Fatalf("failed to parse node address for %v, %v", node.Name, err)
		return "", err
	}

	//check whether the raven-agent is on the node
	if isEdgeNode(node) && dc.hasRavenAgentPod(node.Name) {
		ip, err = dc.getRavenInternalServiceIP(true)
	}

	return ip, err
}

func (dc *dnsRecordController) onDeleteNode(node *corev1.Node) error {
	dnsRecords, err := dc.getCurrentRavenDNSRecord()
	if err != nil {
		return err
	}

	dnsRecordsMap := strToMap(&dnsRecords)
	if _, ok := dnsRecordsMap[node.Name]; ok {
		delete(dnsRecordsMap, node.Name)
		dnsRecordsStr := mapToStr(&dnsRecordsMap)
		err = dc.updateCurrentRavenDNSRecord(dnsRecordsStr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dc *dnsRecordController) onAddService() error {
	dc.syncDNSRecordAsWhole()
	return nil
}

func (dc *dnsRecordController) onUpdateService() error {
	dc.syncDNSRecordAsWhole()
	return nil
}

func (dc *dnsRecordController) onAddPod(pod *corev1.Pod) error {
	return dc.updateRecordAccordingPod(pod, false)
}

func (dc *dnsRecordController) onUpdatePod(pod *corev1.Pod) error {
	return dc.updateRecordAccordingPod(pod, false)
}

func (dc *dnsRecordController) onDeletePod(pod *corev1.Pod) error {
	return dc.updateRecordAccordingPod(pod, true)
}

func (dc *dnsRecordController) getCurrentRavenDNSRecord() (string, error) {
	cm, err := dc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).
		Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	data, ok := cm.Data[RavenDNSRecordNodeDataKey]
	if !ok || len(data) == 0 {
		return "", nil
	}

	return data, nil
}

func (dc *dnsRecordController) updateCurrentRavenDNSRecord(newRecords string) error {
	cm, err := dc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).
		Get(context.TODO(), ravenDNSRecordConfigMapName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[RavenDNSRecordNodeDataKey] = newRecords
	if _, err := dc.client.CoreV1().ConfigMaps(RavenProxyResourceNamespace).
		Update(context.TODO(), cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update configmap %v/%v, %w",
			RavenProxyResourceNamespace, ravenDNSRecordConfigMapName, err)
	}
	return nil
}

func (dc *dnsRecordController) hasRavenAgentPod(nodeName string) bool {

	pods, err := dc.getPodsAssignedToNode(nodeName)
	if err != nil {
		klog.Errorf("failed to get raven agent pod that running in %s, err: %w", nodeName, err)
		return false
	}
	if len(pods) < 1 {
		return false
	}
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodRunning {
			return true
		}
	}
	klog.Infof("there is no running raven agent pod in node %s", nodeName)
	return false
}

func (dc *dnsRecordController) updateRecordAccordingPod(pod *corev1.Pod, isDelete bool) error {
	node, err := dc.dnsSharedInformer.nodeLister.Get(pod.Spec.NodeName)
	if err != nil {
		return err
	}

	ip, err := getNodeHostIP(node)
	if err != nil {
		klog.Fatalf("failed to parse node address for %v, %v", node.Name, err)
		return err
	}

	if isEdgeNode(node) && !isDelete {
		ip, err = dc.getRavenInternalServiceIP(true)
		if err != nil {
			return err
		}
	}

	dnsRecords, err := dc.getCurrentRavenDNSRecord()
	if err != nil {
		return err
	}
	dnsRecordsMap := strToMap(&dnsRecords)
	dnsRecordsMap[node.Name] = ip
	dnsRecordsStr := mapToStr(&dnsRecordsMap)
	return dc.updateCurrentRavenDNSRecord(dnsRecordsStr)
}
