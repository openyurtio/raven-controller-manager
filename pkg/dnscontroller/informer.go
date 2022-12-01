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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// RegisterInformersForDNS registers shared informers that tunnel server use.
func RegisterInformersForDNS(informerFactory informers.SharedInformerFactory) {
	// register filtered service informers
	informerFactory.InformerFor(&corev1.Service{}, newServiceInformer)

	// register filtered pod informers
	informerFactory.InformerFor(&corev1.Pod{}, newPodInformer)
}

// newServiceInformer creates a shared index informers that returns services related to yurttunnel
func newServiceInformer(client clientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {

	tweakListOptions := func(options *metav1.ListOptions) {
		options.LabelSelector = fmt.Sprintf("%v=%v", ravenProxyServiceLabelKey, ravenProxyServiceLabelValue)
		options.FieldSelector = fmt.Sprintf("%s=%s", "metadata.name", RavenProxyInternalServiceName)
	}
	serviceIndexers := cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}
	return coreinformers.NewFilteredServiceInformer(client, RavenProxyResourceNamespace, resyncPeriod, serviceIndexers, tweakListOptions)
}

// newPodInformer creates a shared index informers that returns only interested pods
func newPodInformer(cs clientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {

	selector := fmt.Sprintf("%s=%s", RavenAgentPodLabelKey, RavenAgentPodLabelValue)
	tweakListOptions := func(options *metav1.ListOptions) {
		options.LabelSelector = selector
	}

	podIndexers := cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		NodeNameKeyIndex:     NodeNameIndexFunc,
	}

	return coreinformers.NewFilteredPodInformer(cs, RavenProxyResourceNamespace, resyncPeriod, podIndexers, tweakListOptions)
}

func NodeNameIndexFunc(obj interface{}) ([]string, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return []string{}, nil
	}
	if len(pod.Spec.NodeName) == 0 {
		return []string{}, nil
	}
	return []string{pod.Spec.NodeName}, nil
}
