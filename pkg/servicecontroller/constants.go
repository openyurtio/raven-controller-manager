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

import "github.com/openyurtio/raven-controller-manager/pkg/dnscontroller"

const (
	RavenProxyInternalServiceName = "x-raven-server-internal-svc"
	RavenProxyResourceNamespace   = "kube-system"

	RavenProxyDNATConfigMapName = "%s-tunnel-server-cfg"
	RavenServerLabelKey         = "%s-app"
	RavenServerLabelValue       = "%s-tunnel-server"

	RavenServerHTTPProxyPorts  = "http-proxy-ports"
	RavenServerHTTPSProxyPorts = "https-proxy-ports"
	RavenExtendedPort          = "extend"

	SecurePort       = "10263"
	InsecurePort     = "10264"
	KubeletHTTPSPort = "10250"
	KubeletHTTPPort  = "10255"
	MinPort          = 1
	MaxPort          = 65535
	MaxRetries       = 15
	MinSyncPeriod    = 30
)

var (
	ravenProxyPortConfigMapName = GetRavenProxyPortConfigMapName()
	ravenProxyServiceLabelKey   = dnscontroller.GetRavenProxyServiceLabelKey()
	ravenProxyServiceLabelValue = dnscontroller.GetRavenProxyServiceLabelValue()
	ravenServerLabelKey         = GetRavenServerLabelKey()
	ravenServerLabelValue       = GetRavenServerLabelValue()
)
