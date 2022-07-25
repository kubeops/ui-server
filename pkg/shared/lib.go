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
	"bytes"
	"net"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	meta_util "kmodules.xyz/client-go/meta"
	"kmodules.xyz/client-go/tools/clusterid"
	corev1alpha1 "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
)

var BufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

type matcherType int

const (
	empty matcherType = iota
	anyKind
	specificKind
)

type GroupKindSelector struct {
	everything bool
	groups     map[string]matcherType
	groupKinds map[schema.GroupKind]matcherType
}

func NewGroupKindSelector(s labels.Selector) GroupKindSelector {
	if s == nil || s.Empty() {
		return GroupKindSelector{everything: true}
	}

	gks := GroupKindSelector{
		groups:     map[string]matcherType{},
		groupKinds: map[schema.GroupKind]matcherType{},
	}
	if requirements, selectable := s.Requirements(); selectable {
		for _, r := range requirements {
			if r.Key() == "k8s.io/group" && (r.Operator() == selection.In || r.Operator() == selection.Equals) {
				for _, group := range r.Values().UnsortedList() {
					gks.groups[group] = anyKind
				}
				break
			}
		}
		for _, r := range requirements {
			if r.Key() == "k8s.io/group-kind" && (r.Operator() == selection.In || r.Operator() == selection.Equals) {
				for _, str := range r.Values().UnsortedList() {
					gk := schema.ParseGroupKind(str)
					gks.groups[gk.Group] = specificKind
					gks.groupKinds[gk] = empty
				}
				break
			}
		}
	}
	return gks
}

func (s GroupKindSelector) Matches(gk schema.GroupKind) bool {
	if s.everything {
		return true
	}
	if v, ok := s.groups[gk.Group]; !ok {
		return false
	} else if v == anyKind {
		return true
	}
	_, ok := s.groupKinds[gk]
	return ok
}

func GetKubernetesInfo(cfg *rest.Config, kc kubernetes.Interface) (*corev1alpha1.KubernetesInfo, error) {
	var si corev1alpha1.KubernetesInfo

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
		si.ControlPlane = &corev1alpha1.ControlPlaneInfo{
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

var (
	podGVR     = schema.GroupVersionResource{Version: "v1", Resource: "Pods"}
	podviewGVR = corev1alpha1.GroupVersion.WithResource(corev1alpha1.ResourcePodViews)
)

func IsPod(gvr schema.GroupVersionResource) bool {
	return gvr == podGVR || gvr == podviewGVR
}
