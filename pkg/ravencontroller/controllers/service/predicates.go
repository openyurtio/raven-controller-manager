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

package service

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
)

// ServicePredicates filters certain notable change events for services
// before enqueuing the keys.
type ServicePredicates struct {
	predicate.Funcs
	log logr.Logger
}

func (n ServicePredicates) Update(e event.UpdateEvent) bool {
	return isManagedService(e.ObjectNew) || isManagedService(e.ObjectOld)
}

func (n ServicePredicates) Create(e event.CreateEvent) bool {
	return isManagedService(e.Object)
}

func (n ServicePredicates) Delete(e event.DeleteEvent) bool {
	return isManagedService(e.Object)
}

func (n ServicePredicates) Generic(e event.GenericEvent) bool {
	return isManagedService(e.Object)
}

func isManagedService(o client.Object) bool {
	if o != nil {
		ep, ok := o.(*corev1.Service)
		if ok {
			return ep.Name == v1alpha1.ServiceNamespacedName.Name && ep.Namespace == v1alpha1.ServiceNamespacedName.Namespace
		}
	}
	return false
}
