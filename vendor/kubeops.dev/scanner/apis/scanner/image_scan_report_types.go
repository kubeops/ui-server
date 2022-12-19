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

package scanner

import (
	"kubeops.dev/scanner/apis/trivy"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageScanReport defines the vulnerability report a Docker image reference.

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ImageScanReport struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec   ImageScanReportSpec
	Status ImageScanReportStatus
}

type ImageScanReportSpec struct {
	Image ImageReference
}

type ImageReference struct {
	Name   string
	Tag    string
	Digest string
}

type ImageScanReportStatus struct {
	LastChecked    trivy.Time
	TrivyDBVersion string
	Report         trivy.SingleReport
}

// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ImageScanReportList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []ImageScanReport
}
