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

package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceKindFalcoEvent = "FalcoEvent"
	ResourceFalcoEvent     = "falcoevent"
	ResourceFalcoEvents    = "falcoevents"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type FalcoEvent struct {
	metav1.TypeMeta `json:",inline"`
	// Name will be formed by hashing the ImageRef + Tag + Digest
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec describes the attributes for the Image Scan SingleReport
	Spec FalcoEventSpec `json:"spec,omitempty"`
}

type FalcoEventSpec struct {
	UUID         string               `json:"uuid,omitempty"`
	Output       string               `json:"output"`
	Priority     string               `json:"priority"`
	Rule         string               `json:"rule"`
	Time         metav1.Time          `json:"time"`
	OutputFields apiextensionsv1.JSON `json:"outputFields"`
	Source       string               `json:"source"`
	Tags         []string             `json:"tags,omitempty"`
	Hostname     string               `json:"hostname,omitempty"`
	Nodename     string               `json:"nodename,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FalcoEventList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of Memcached TPR objects
	Items []FalcoEvent `json:"items,omitempty"`
}
