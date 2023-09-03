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

package falco

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FalcoEvent defines the vulnerability report a Docker image reference.

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type FalcoEvent struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec FalcoEventSpec
}

type FalcoEventSpec struct {
	UUID         string
	Output       string
	Priority     string
	Rule         string
	Time         metav1.Time
	OutputFields apiextensionsv1.JSON
	Source       string
	Tags         []string
	Hostname     string
	Nodename     string
}

// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type FalcoEventList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []FalcoEvent
}
