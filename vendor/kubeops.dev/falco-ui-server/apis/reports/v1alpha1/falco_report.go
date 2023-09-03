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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
)

const (
	ResourceKindFalcoReport = "FalcoReport"
	ResourceFalcoReport     = "falcoreport"
	ResourceFalcoReports    = "falcoreports"
)

// +genclient
// +genclient:nonNamespaced
// +genclient:onlyVerbs=create
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type FalcoReport struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	Request *FalcoReportRequest `json:"request,omitempty"`
	// +optional
	Response *FalcoReportResponse `json:"response,omitempty"`
}

type FalcoReportRequest struct {
	kmapi.ObjectInfo `json:",inline"`
}

type FalcoReportResponse struct {
	FalcoEventRefs []string `json:"falcoEventRefs"`
}
