/*
 * Copyright 2022 The OpenYurt Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controllers

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	ravenv1alpha1 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
)

// PodChangedPredicates filters certain notable change events for pods
// before enqueuing the keys.
type PodChangedPredicates struct {
	predicate.Funcs
	log logr.Logger
}

func (p PodChangedPredicates) Create(e event.CreateEvent) bool {
	if e.Object == nil {
		p.log.Error(nil, "Create event has no object to create", "event", e)
		return false
	}
	pod := e.Object.(*corev1.Pod)
	return isAgentPod(pod)
}

func (p PodChangedPredicates) Delete(e event.DeleteEvent) bool {
	if e.Object == nil {
		p.log.Error(nil, "Delete event has no object to delete", "event", e)
		return false
	}
	pod := e.Object.(*corev1.Pod)
	return isAgentPod(pod)
}

// Update implements default UpdateEvent filter for validating notable change.
// Notable change including:
// * PodReady condition change;
func (p PodChangedPredicates) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil {
		p.log.Error(nil, "Update event has no old object to update", "event", e)
		return false
	}
	if e.ObjectNew == nil {
		p.log.Error(nil, "Update event has no new object to update", "event", e)
		return false
	}
	oldObj := e.ObjectOld.(*corev1.Pod)
	newObj := e.ObjectNew.(*corev1.Pod)

	// check if PodReady condition changed
	podStatusChanged := func(oldObj, newObj *corev1.Pod) bool {
		return isPodReady(*oldObj) != isPodReady(*newObj)
	}
	return isAgentPod(oldObj) && isAgentPod(newObj) && podStatusChanged(oldObj, newObj)
}

func (p PodChangedPredicates) Generic(e event.GenericEvent) bool {
	if e.Object == nil {
		p.log.Error(nil, "Generic event has no object", "event", e)
		return false
	}
	pod := e.Object.(*corev1.Pod)
	return isAgentPod(pod)
}

func isAgentPod(pod *corev1.Pod) bool {
	labels := pod.GetLabels()
	if labels == nil {
		return false
	}
	if value, ok := labels[ravenv1alpha1.LabelAgentPod]; ok && value == "true" {
		return true
	}
	return false
}
