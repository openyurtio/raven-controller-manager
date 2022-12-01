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
	"net"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/openyurtio/raven-controller-manager/pkg/projectinfo"
)

// GetRavenDNSRecordConfigMapName get the configmap named yurt-tunnel-nodes
func GetRavenDNSRecordConfigMapName() string {
	return fmt.Sprintf(RavenDNSRecordConfigMapName,
		strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
}

// GetRavenProxyServiceLabelKey get the label key of raven service : yurt-app
func GetRavenProxyServiceLabelKey() string {
	return fmt.Sprintf(RavenProxyServiceLabelKey,
		strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
}

// GetRavenProxyServiceLabelValue get the label value of raven service : yurt-tunnel-server
func GetRavenProxyServiceLabelValue() string {
	return fmt.Sprintf(RavenProxyServiceLabelValue,
		strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
}

func isEdgeNode(node *corev1.Node) bool {
	f, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
	if ok && f == "true" {
		return true
	}
	return false
}

func formatDNSRecord(ip, host string) string {
	return fmt.Sprintf("%s\t%s", ip, host)
}

func getNodeHostIP(node *corev1.Node) (string, error) {
	// get InternalIPs first and then ExternalIPs
	allIPs := make([]net.IP, 0, len(node.Status.Addresses))
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			ip := net.ParseIP(addr.Address)
			if ip != nil {
				allIPs = append(allIPs, ip)
				break
			}
		}
	}
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			ip := net.ParseIP(addr.Address)
			if ip != nil {
				allIPs = append(allIPs, ip)
				break
			}
		}
	}
	if len(allIPs) == 0 {
		return "", fmt.Errorf("host IP unknown; known addresses: %v", node.Status.Addresses)
	}

	return allIPs[0].String(), nil
}

func strToMap(src *string) (dst map[string]string) {
	dst = make(map[string]string, 0)
	if len(*src) == 0 {
		return
	}
	records := strings.Split(*src, "\n")
	for _, record := range records {
		data := strings.Split(record, "\t")
		if len(data) != 2 {
			continue
		}
		dst[data[1]] = data[0]
	}
	return
}

func mapToStr(src *map[string]string) (dst string) {
	if len(*src) == 0 {
		return ""
	}
	records := make([]string, 0, len(*src))
	for nodeName, address := range *src {
		records = append(records, formatDNSRecord(address, nodeName))
	}
	sort.Strings(records)
	return strings.Join(records, "\n")
}
