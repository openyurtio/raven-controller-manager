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

// Event reason.
const (
	// EventActiveEndpointsElected is the event indicating new active endpoint were elected.
	EventActiveEndpointsElected = "ActiveEndpointsElected"
	// EventActiveEndpointsLost is the event indicating the active endpoints were lost.
	EventActiveEndpointsLost = "ActiveEndpointsLost"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewaySpec defines the desired state of Gateway
type GatewaySpec struct {
	// NodeSelector is a label query over nodes that managed by the gateway.
	// The nodes in the same gateway should share same layer 3 network.
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	// TODO add a field to configure using vxlan or host-gw for inner gateway communication?
	// Endpoints is a list of available Endpoint.
	Endpoints []Endpoint `json:"endpoints"`
	// Replicas is the desired number of active endpoints.
	Replicas *int `json:"replicas,omitempty"`
	// Central indicates a gateway can be the central gateway
	Central bool `json:"central,omitempty"`
}

// Endpoint stores all essential data for establishing the VPN tunnel.
// TODO add priority field?
type Endpoint struct {
	// NodeName is the Node hosting this endpoint.
	NodeName string            `json:"nodeName"`
	UnderNAT bool              `json:"underNAT,omitempty"`
	PublicIP string            `json:"publicIP,omitempty"`
	Config   map[string]string `json:"config,omitempty"`
}

// NodeInfo stores information of node managed by Gateway.
type NodeInfo struct {
	NodeName  string   `json:"nodeName"`
	PrivateIP string   `json:"privateIP"`
	Subnets   []string `json:"subnets"`
}

// Forward stores the traffic that the central gateway is responsible for forwarding
type Forward struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ActiveEndpoint defines the active endpoint that selected by controller manager.
type ActiveEndpoint struct {
	// Endpoint is the reference of the active endpoint.
	Endpoint *Endpoint `json:"endpoint"`
	// Nodes contains all information of nodes managed by active endpoint.
	Nodes []NodeInfo `json:"nodes"`
	// Forwards contains all vpn connections this active endpoint managed by.
	// This field is used only when the gateway is the central gateway.
	Forwards []Forward `json:"forwards,omitempty"`
	// Healthy indicates whether this active endpoint is healthy
	Healthy bool `json:"healthy"`
}

// GatewayStatus defines the observed state of Gateway
type GatewayStatus struct {
	// ActiveEndpoints is a list of active endpoints
	ActiveEndpoints []*ActiveEndpoint `json:"activeEndpoints,omitempty"`
	// Central indicates a gateway is the central gateway
	Central bool `json:"central,omitempty"`
}

//+genclient
//+genclient:nonNamespaced
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="ActiveEndpoints",type=string,JSONPath=`.status.activeEndpoints[*].endpoint.nodeName`
//+kubebuilder:printcolumn:name="Central",type=boolean,JSONPath=`.status.central`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

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
