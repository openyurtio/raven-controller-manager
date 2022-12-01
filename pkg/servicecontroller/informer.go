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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	ravenv1alpha1 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
	racenclient "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/clientset/versioned"
	raveninformers "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/informers/externalversions"
	ravenv1aplph1informers "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/informers/externalversions/raven/v1alpha1"
)

func RegisterRavenInformersForService(ravenInformerFactory raveninformers.SharedInformerFactory) {
	//register filtered gateway informers
	ravenInformerFactory.InformerFor(&ravenv1alpha1.Gateway{}, newGatewayInformer)

}

func newGatewayInformer(client racenclient.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	tweakListOptions := func(options *metav1.ListOptions) {
		options.LabelSelector = fmt.Sprintf("%v=%v", ravenServerLabelKey, ravenServerLabelValue)
	}
	gatewayIndexers := cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}
	return ravenv1aplph1informers.NewFilteredGatewayInformer(client, resyncPeriod, gatewayIndexers, tweakListOptions)
}

// RegisterInformersForTunnelServer registers shared informers that tunnel server use.
func RegisterInformersForService(informerFactory informers.SharedInformerFactory) {

	// register filtered service informers
	informerFactory.InformerFor(&corev1.Service{}, newServiceInformer)

	// register filtered pod informers
	informerFactory.InformerFor(&corev1.ConfigMap{}, newConfigMapInformer)
}

// newServiceInformer creates a shared index informers that returns services related to yurttunnel
func newServiceInformer(client clientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {

	tweakListOptions := func(options *metav1.ListOptions) {
		options.LabelSelector = fmt.Sprintf("%v=%v", ravenProxyServiceLabelKey, ravenProxyServiceLabelValue)
	}
	serviceIndexers := cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}
	return coreinformers.NewFilteredServiceInformer(client, RavenProxyResourceNamespace, resyncPeriod, serviceIndexers, tweakListOptions)
}

// newConfigMapInformer creates a shared index informers that returns only interested configmaps
func newConfigMapInformer(client clientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	selector := fmt.Sprintf("metadata.name=%v", ravenProxyPortConfigMapName)
	tweakListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = selector
	}
	configmapIndexers := cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}
	return coreinformers.NewFilteredConfigMapInformer(client, RavenProxyResourceNamespace, resyncPeriod, configmapIndexers, tweakListOptions)
}
