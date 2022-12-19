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
	"kubeops.dev/scanner/apis/trivy"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceKindImageScanReport = "ImageScanReport"
	ResourceImageScanReport     = "imagescanreport"
	ResourceImageScanReports    = "imagescanreports"
)

// ImageScanReport defines the vulnerability report a Docker image reference.

// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ImageScanReport struct {
	metav1.TypeMeta `json:",inline"`
	// Name will be formed by hashing the ImageRef + Tag + Digest
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec describes the attributes for the Image Scan SingleReport
	Spec ImageScanReportSpec `json:"spec,omitempty"`

	// Status holds all the SingleReport-related details of the specified image
	Status ImageScanReportStatus `json:"status,omitempty"`
}

type ImageScanReportSpec struct {
	Image ImageReference `json:"image"`
}

type ImageReference struct {
	Name   string `json:"name"`
	Tag    string `json:"tag,omitempty"`
	Digest string `json:"digest,omitempty"`
}

type ImageScanReportStatus struct {
	// When the referred image was checked for the last time
	// +optional
	LastChecked trivy.Time `json:"lastChecked,omitempty"`

	// which TrivyDBVersion was used when the last check
	// +optional
	TrivyDBVersion string `json:"trivyDBVersion,omitempty"`

	// This is the actual trivy Report
	// +optional
	Report trivy.SingleReport `json:"report,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ImageScanReportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of Memcached TPR objects
	Items []ImageScanReport `json:"items,omitempty"`
}
