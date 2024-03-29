/*
Copyright AppsCode Inc. and Contributors

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

package graph

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
)

const (
	// CostFactorOfInAppFiltering = 4 means, we assume that the cost of listing all resources and
	// filtering them in the app (instead of using kube-apiserver) is 5x of that via label based selection
	CostFactorOfInAppFiltering = 4

	MetadataNamespace      = "metadata.namespace"
	MetadataNamespaceQuery = "{." + MetadataNamespace + "}"
	MetadataLabels         = "metadata.labels"
	MetadataNameQuery      = "{.metadata.name}"
)

type Edge struct {
	Src        schema.GroupVersionKind
	Dst        schema.GroupVersionKind
	W          uint64
	Connection rsapi.ResourceConnectionSpec
	Forward    bool
}

type AdjacencyMap map[schema.GroupVersionKind]*Edge

// Types of Selectors

// metav1.LabelSelector
// *metav1.LabelSelector

// map[string]string

// ref: https://github.com/coreos/prometheus-operator/blob/cc584ecfa08d2eb95ba9401f116e3a20bf71be8b/pkg/apis/monitoring/v1/types.go#L578
// NamespaceSelector is a selector for selecting either all namespaces or a
// list of namespaces.
// +k8s:openapi-gen=true
type NamespaceSelector struct {
	// Boolean describing whether all namespaces are selected in contrast to a
	// list restricting them.
	Any bool `json:"any,omitempty"`
	// List of namespace names.
	MatchNames []string `json:"matchNames,omitempty"`

	// TODO(fabxc): this should embed metav1.LabelSelector eventually.
	// Currently the selector is only used for namespaces which require more complex
	// implementation to support label selections.
}

// ResourceRef contains information that points to the resource being used
type ResourceRef struct {
	// Name is the name of resource being referenced
	Name string `json:"name"`
	// Namespace is the namespace of resource being referenced
	Namespace string `json:"namespace,omitempty"`
	// Kind is the type of resource being referenced
	Kind string `json:"kind,omitempty"`
	// APIGroup is the group for the resource being referenced
	APIGroup string `json:"apiGroup,omitempty"`
}

func fields(path string) []string {
	// TODO(tamal): support escape of . using \
	return strings.Split(strings.Trim(path, "."), ".")
}

func contains(arr []string, item string) bool {
	for _, v := range arr {
		if v == item {
			return true
		}
	}
	return false
}
