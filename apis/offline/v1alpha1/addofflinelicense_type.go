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
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceKindAddOfflineLicense = "AddOfflineLicense"
	ResourceAddOfflineLicense     = "addofflinelicense"
	ResourceAddOfflineLicenses    = "addofflinelicenses"
)

// +genclient
// +genclient:nonNamespaced
// +genclient:onlyVerbs=create
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AddOfflineLicense struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	Request *AddOfflineLicenseRequest `json:"request,omitempty"`
	// +optional
	Response *AddOfflineLicenseResponse `json:"response,omitempty"`
}

type AddOfflineLicenseRequest struct {
	Namespace string `json:"namespace"`
	License   string `json:"license"`
}

type AddOfflineLicenseResponse struct {
	// +optional
	SecretKeyRef *core.SecretKeySelector `json:"secretKeyRef,omitempty"`
}
