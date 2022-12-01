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

	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	ravenv1alpha1 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
)

func (sc *serviceController) addService(obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}
	if svc.DeletionTimestamp != nil {
		return
	}
	if svc.Name != RavenProxyInternalServiceName {
		return
	}
	klog.V(2).Infof("enqueue service add event for %v/%v", svc.Namespace, svc.Name)
	sc.enqueue(ServiceAdd, svc)
}

func (sc *serviceController) addConfigMap(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	if cm.DeletionTimestamp != nil {
		sc.deleteConfigMap(cm)
		return
	}
	klog.V(2).Infof("enqueue configmap add event for %v/%v", cm.Namespace, cm.Name)
	sc.enqueue(ConfigMapAdd, cm)
}

func (sc *serviceController) updateConfigMap(oldObj, newObj interface{}) {
	oldConfigMap, ok := oldObj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	newConfigMap, ok := newObj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	if reflect.DeepEqual(oldConfigMap.Data, newConfigMap.Data) {
		return
	}

	klog.V(2).Infof("enqueue configmap update event for %v/%v, will sync raven server svc", newConfigMap.Namespace, newConfigMap.Name)
	sc.enqueue(ConfigMapUpdate, newConfigMap)
}

func (sc *serviceController) deleteConfigMap(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("can not get object from tombstone %#v", obj))
			return
		}
		cm, ok = tombstone.Obj.(*corev1.ConfigMap)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object is not a comfigmap %#v", obj))
			return
		}
	}
	klog.V(2).Infof("enqueue configmap delete event for %v/%v", cm.Namespace, cm.Name)
	sc.enqueue(ConfigMapDelete, cm)
}

func (sc *serviceController) addGateWay(obj interface{}) {
	gw, ok := obj.(*ravenv1alpha1.Gateway)
	if !ok {
		return
	}
	if gw.Status.ActiveEndpoint == nil || len(gw.Status.ActiveEndpoint.NodeName) < 1 {
		klog.V(2).Infof("the gateway node has not been selected in gateway %s", gw.Name)
		return
	}
	klog.V(2).Infof("enqueue gateway add event for %v", gw.Name)
	sc.enqueue(GatewayAdd, gw)
}

func (sc *serviceController) updateGateWay(oldObj, newObj interface{}) {
	oldGw, ok := oldObj.(*ravenv1alpha1.Gateway)
	if !ok {
		return
	}

	newGw, ok := newObj.(*ravenv1alpha1.Gateway)
	if !ok {
		return
	}

	if oldGw.Status.ActiveEndpoint == nil || len(oldGw.Status.ActiveEndpoint.NodeName) < 1 {
		return
	}
	klog.V(2).Infof("enqueue gateway delete event for %v", oldGw.Name)
	sc.enqueue(GatewayDelete, oldGw)

	if newGw.Status.ActiveEndpoint == nil || len(newGw.Status.ActiveEndpoint.NodeName) < 1 {
		return
	}
	klog.V(2).Infof("enqueue gateway update event for %v", newGw.Name)
	sc.enqueue(GatewayUpdate, newGw)

}

func (sc *serviceController) deleteGateWay(obj interface{}) {
	gw, ok := obj.(*ravenv1alpha1.Gateway)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("can not get object from tombstone %#v", obj))
			return
		}
		gw, ok = tombstone.Obj.(*ravenv1alpha1.Gateway)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object is not a gateway %#v", obj))
			return
		}
	}
	klog.V(2).Infof("enqueue gateway delete event for %v", gw.Name)
	sc.enqueue(GatewayDelete, gw)
}
