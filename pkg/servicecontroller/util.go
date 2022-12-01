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
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/openyurtio/raven-controller-manager/pkg/projectinfo"
)

// GetRavenProxyPortConfigMapName get the configmap named yurt-raven-server-cfg
func GetRavenProxyPortConfigMapName() string {
	return fmt.Sprintf(RavenProxyDNATConfigMapName,
		strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
}

// GetRavenServerLabelKey get the label key of raven service : yurt-app
func GetRavenServerLabelKey() string {
	return fmt.Sprintf(RavenServerLabelKey,
		strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
}

// GetRavenServerLabelValue get the label key of raven service : yurt-tunnel-server
func GetRavenServerLabelValue() string {
	return fmt.Sprintf(RavenServerLabelValue,
		strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
}

// GetProxyPortsAndMappings get proxy ports and port mappings from specified configmap
func getProxyPortsMap(cm *corev1.ConfigMap, insecureListenPort, secureListenPort string) map[string]string {
	portsMap := make(map[string]string)

	// resolve http-proxy-port field
	for _, port := range processPort(cm.Data[RavenServerHTTPProxyPorts]) {
		portsMap[port] = insecureListenPort
	}

	// resolve https-proxy-port field
	for _, port := range processPort(cm.Data[RavenServerHTTPSProxyPorts]) {
		portsMap[port] = secureListenPort
	}

	// cleanup 10250/10255 mappings
	delete(portsMap, KubeletHTTPSPort)
	delete(portsMap, KubeletHTTPPort)

	return portsMap
}

// processPort parse the specified ports setting and return ports slice.
func processPort(portsStr string) []string {
	ports := make([]string, 0)
	if len(strings.TrimSpace(portsStr)) == 0 {
		return ports
	}

	portsSubStr := strings.Split(portsStr, ",")
	for _, port := range portsSubStr {
		proxyPort := strings.TrimSpace(port)
		if len(proxyPort) != 0 {
			portInt, err := strconv.Atoi(proxyPort)
			if err != nil {
				klog.Errorf("failed to parse port %s, %v", port, err)
				continue
			} else if portInt < MinPort || portInt > MaxPort {
				klog.Errorf("port %s is not invalid port(should be range 1~65535)", port)
				continue
			}

			ports = append(ports, proxyPort)
		}
	}
	return ports
}

func updateServicePortsRecord(oldServicePortMap, newServicePortMap map[string]corev1.ServicePort) (bool, []corev1.ServicePort) {
	var isChanged = false
	svcPortsRecord := make([]corev1.ServicePort, 0)

	equal := func(p1, p2 *corev1.ServicePort) bool {
		if p1.Name != p2.Name {
			return false
		}
		if p1.Port != p2.Port {
			return false
		}
		if p1.Protocol != p2.Protocol {
			return false
		}
		if p1.TargetPort != p2.TargetPort {
			return false
		}
		return true
	}

	for portKey, newSvcPort := range newServicePortMap {
		oldSvcPort, ok := oldServicePortMap[portKey]
		if !ok {
			oldServicePortMap[portKey] = newSvcPort
			isChanged = true
			continue
		}
		if !equal(&oldSvcPort, &newSvcPort) {
			oldServicePortMap[portKey] = newSvcPort
			isChanged = true
			continue
		}
	}

	for portKey, servicePort := range oldServicePortMap {
		_, ok := newServicePortMap[portKey]
		if !ok &&
			strings.HasPrefix(portKey, string(corev1.ProtocolTCP)) &&
			strings.HasPrefix(servicePort.Name, RavenExtendedPort) {
			isChanged = true
			continue
		}
		svcPortsRecord = append(svcPortsRecord, servicePort)
	}
	return isChanged, svcPortsRecord
}
