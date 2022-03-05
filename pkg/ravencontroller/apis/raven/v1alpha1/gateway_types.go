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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// All supported backend.
const (
	BackendLibreswan = "libreswan"
)

// Event reason.
const (
	// EventActiveEndpointElected is the event indicating a new active endpoint is elected.
	EventActiveEndpointElected = "ActiveEndpointElected"
	// EventActiveEndpointLost is the event indicating the active endpoint is lost.
	EventActiveEndpointLost = "ActiveEndpointLost"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewaySpec defines the desired state of Gateway
type GatewaySpec struct {
	// NodeSelector is a label query over nodes that managed by the gateway.
	// The nodes in the same gateway should share same layer 3 network.
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	// Backend is the VPN tunnel implementation, only "libreswan" is supported currently.
	// The default value "libreswan" will be used if empty.
	Backend string `json:"backend"`
	// TODO add a field to configure using vxlan or host-gw for inner gateway communication?
	// Endpoints is a list of available Endpoint.
	Endpoints []Endpoint `json:"endpoints,omitempty"`
}

// Endpoint stores all essential data for establishing the VPN tunnel.
// TODO add priority field?
type Endpoint struct {
	// NodeName is the Node hosting this endpoint.
	NodeName   string            `json:"nodeName"`
	PrivateIP  string            `json:"privateIP"`
	PublicIP   string            `json:"publicIP"`
	NATEnabled bool              `json:"natEnabled,omitempty"`
	Config     map[string]string `json:"config,omitempty"`
}

// GatewayStatus defines the observed state of Gateway
type GatewayStatus struct {
	// Subnets contains all the subnets in the gateway.
	Subnets []string `json:"subnets,omitempty"`
	// ActiveEndpoint is the reference of the active endpoint.
	ActiveEndpoint *Endpoint `json:"activeEndpoint,omitempty"`
}

//+genclient
//+genclient:nonNamespaced
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="ActiveEndpoint",type=string,JSONPath=`.status.activeEndpoint.nodeName`

// Gateway is the Schema for the gateways API
type Gateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewaySpec   `json:"spec,omitempty"`
	Status GatewayStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayList contains a list of Gateway
type GatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Gateway{}, &GatewayList{})
}
