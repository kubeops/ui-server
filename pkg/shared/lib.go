/*
Copyright AppsCode Inc. and Contributors.

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

package shared

import (
	"net"
	"strings"

	uiv1alpha1 "kubeops.dev/ui-server/apis/ui/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	meta_util "kmodules.xyz/client-go/meta"
	"kmodules.xyz/client-go/tools/clusterid"
)

func GetAPIGroups(s labels.Selector) sets.String {
	g, found := s.RequiresExactMatch("k8s.io/group")
	if found {
		return sets.NewString(g)
	}

	requirements, selectable := s.Requirements()
	if selectable {
		for _, r := range requirements {
			if r.Key() == "k8s.io/group" && r.Operator() == selection.In {
				return r.Values()
			}
		}
	}
	return sets.NewString()
}

func GetKubernetesInfo(cfg *rest.Config, kc kubernetes.Interface) (*uiv1alpha1.KubernetesInfo, error) {
	var si uiv1alpha1.KubernetesInfo

	var err error
	si.ClusterName = clusterid.ClusterName()
	si.ClusterUID, err = clusterid.ClusterUID(kc.CoreV1().Namespaces())
	if err != nil {
		return nil, err
	}
	si.Version, err = kc.Discovery().ServerVersion()
	if err != nil {
		return nil, err
	}

	cert, err := meta_util.APIServerCertificate(cfg)
	if err != nil {
		return nil, err
	} else {
		si.ControlPlane = &uiv1alpha1.ControlPlaneInfo{
			NotBefore: metav1.NewTime(cert.NotBefore),
			NotAfter:  metav1.NewTime(cert.NotAfter),
			// DNSNames:       cert.DNSNames,
			EmailAddresses: cert.EmailAddresses,
			// IPAddresses:    cert.IPAddresses,
			// URIs:           cert.URIs,
		}

		dnsNames := sets.NewString(cert.DNSNames...)
		ips := sets.NewString()
		if len(cert.Subject.CommonName) > 0 {
			if ip := net.ParseIP(cert.Subject.CommonName); ip != nil {
				if !skipIP(ip) {
					ips.Insert(ip.String())
				}
			} else {
				dnsNames.Insert(cert.Subject.CommonName)
			}
		}

		for _, host := range dnsNames.UnsortedList() {
			if host == "kubernetes" ||
				host == "kubernetes.default" ||
				host == "kubernetes.default.svc" ||
				strings.HasSuffix(host, ".svc.cluster.local") ||
				host == "localhost" ||
				!strings.ContainsRune(host, '.') {
				dnsNames.Delete(host)
			}
		}
		si.ControlPlane.DNSNames = dnsNames.List()

		for _, ip := range cert.IPAddresses {
			if !skipIP(ip) {
				ips.Insert(ip.String())
			}
		}
		si.ControlPlane.IPAddresses = ips.List()

		uris := make([]string, 0, len(cert.URIs))
		for _, u := range cert.URIs {
			uris = append(uris, u.String())
		}
		si.ControlPlane.URIs = uris
	}
	return &si, nil
}

func skipIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsMulticast() ||
		ip.IsGlobalUnicast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsLinkLocalUnicast()
}
