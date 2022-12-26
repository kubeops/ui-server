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
	"sync"

	reportsapi "kubeops.dev/scanner/apis/reports/v1alpha1"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	kmapi "kmodules.xyz/client-go/api/v1"
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

var (
	podGVR     = schema.GroupVersionResource{Version: "v1", Resource: "Pods"}
	podviewGVR = corev1alpha1.GroupVersion.WithResource(corev1alpha1.ResourcePodViews)
)

func IsPod(gvr schema.GroupVersionResource) bool {
	return gvr == podGVR || gvr == podviewGVR
}

func IsClusterRequest(req *kmapi.ObjectInfo) bool {
	return req == nil ||
		(req.Resource.Group == "" && req.Resource.Kind == "" && req.Resource.Name == "")
}

func IsImageRequest(req *kmapi.ObjectInfo) bool {
	return req != nil &&
		req.Resource.Group == reportsapi.SchemeGroupVersion.Group &&
		(req.Resource.Kind == "Image" || req.Resource.Name == "images")
}

func IsCVERequest(req *kmapi.ObjectInfo) bool {
	return req != nil &&
		req.Resource.Group == reportsapi.SchemeGroupVersion.Group &&
		(req.Resource.Kind == "CVE" || req.Resource.Name == "cves")
}

func IsClusterCVERequest(req *kmapi.ObjectInfo) bool {
	return IsCVERequest(req) && req.Ref.Namespace == ""
}

func IsNamespaceCVERequest(req *kmapi.ObjectInfo) bool {
	return IsCVERequest(req) && req.Ref.Namespace != ""
}

func IsNamespaceRequest(req *kmapi.ObjectInfo) bool {
	return req != nil &&
		req.Resource.Group == "" &&
		(req.Resource.Kind == "Namespace" || req.Resource.Name == "namespaces")
}

func IsPodRequest(req *kmapi.ObjectInfo) bool {
	return req != nil &&
		req.Resource.Group == "" &&
		(req.Resource.Kind == "Pod" || req.Resource.Name == "pods")
}
