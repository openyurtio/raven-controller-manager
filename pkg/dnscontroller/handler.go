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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

func (dc *dnsRecordController) addNode(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return
	}
	if node.DeletionTimestamp != nil {
		dc.deleteNode(node)
		return
	}
	klog.V(2).Infof("enqueue node add event for %v", node.Name)
	dc.enqueue(NodeAdd, node)
}

func (dc *dnsRecordController) updateNode(oldObj, newObj interface{}) {
	oldNode, ok := oldObj.(*corev1.Node)
	if !ok {
		return
	}
	newNode, ok := newObj.(*corev1.Node)
	if !ok {
		return
	}
	if isEdgeNode(oldNode) == isEdgeNode(newNode) {
		return
	}
	klog.V(2).Infof("enqueue node update event for %v, will update dns record", newNode.Name)
	dc.enqueue(NodeUpdate, newNode)
}

func (dc *dnsRecordController) deleteNode(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("can not get object from tombstone %#v", obj))
			return
		}
		node, ok = tombstone.Obj.(*corev1.Node)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object is not a node %#v", obj))
			return
		}
	}
	klog.V(2).Infof("enqueue node delete event for %v", node.Name)
	dc.enqueue(NodeDelete, node)
}

func (dc *dnsRecordController) addService(obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}
	klog.V(2).Infof("enqueue service add event for %v/%v", svc.Namespace, svc.Name)
	dc.enqueue(ServiceAdd, svc)
}

func (dc *dnsRecordController) updateService(oldObj, newObj interface{}) {
	oldSvc, ok := oldObj.(*corev1.Service)
	if !ok {
		return
	}
	newSvc, ok := newObj.(*corev1.Service)
	if !ok {
		return
	}
	if oldSvc.Spec.ClusterIP != newSvc.Spec.ClusterIP {
		klog.V(2).Infof("enqueue service update event for %v/%v", newSvc.Namespace, newSvc.Name)
		dc.enqueue(ServiceUpdate, newSvc)
	}
}

func (dc *dnsRecordController) deleteService(obj interface{}) {
	// do nothing
}

func (dc *dnsRecordController) addPod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	if len(pod.Spec.NodeName) == 0 {
		klog.V(2).Infof("the raven agent pod %v is not scheduled to the specified node", pod.Name)
		return
	}
	if pod.DeletionTimestamp != nil {
		dc.deletePod(pod)
		return
	}
	klog.V(2).Infof("enqueue raven agent pod add event for %v", pod.Name)
	dc.enqueue(PodAdd, pod)
}

func (dc *dnsRecordController) updatePod(oldObj, newObj interface{}) {
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		return
	}
	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}
	if len(newPod.Spec.NodeName) == 0 {
		klog.V(2).Infof("the raven agent pod %v is not scheduled to the specified node", newPod.Name)
		return
	}

	if oldPod.Spec.NodeName == newPod.Spec.NodeName {
		return
	}
	klog.V(2).Infof("enqueue raven agent pod update event for %v", newPod.Name)
	dc.enqueue(PodUpdate, newPod)
}

func (dc *dnsRecordController) deletePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("can not get object from tombstone %#v", obj))
			return
		}
		pod, ok = tombstone.Obj.(*corev1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object is not a node %#v", obj))
			return
		}
	}
	if len(pod.Spec.NodeName) == 0 {
		klog.V(2).Infof("the raven agent pod %v is not in pod", pod.Name)
		return
	}
	klog.V(2).Infof("enqueue raven agent pod delete event for %v", pod.Name)
	dc.enqueue(PodDelete, pod)
}
