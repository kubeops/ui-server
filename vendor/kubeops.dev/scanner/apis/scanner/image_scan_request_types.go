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

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ImageScanRequest struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec   ImageScanRequestSpec
	Status ImageScanRequestStatus
}

type ImageScanRequestSpec struct {
	Image       string
	PullSecrets []core.LocalObjectReference
	Namespace   string
}

type ImageScanRequestStatus struct {
	ObservedGeneration int64
	Phase              ImageScanRequestPhase
	Image              *ImageDetails
	ReportRef          *ScanReportRef
	Reason             string
}

type ImageVisibility string

const (
	ImagePublic  ImageVisibility = "Public"
	ImagePrivate ImageVisibility = "Private"
)

type ImageScanRequestPhase string

const (
	ImageScanRequestPhasePending    ImageScanRequestPhase = "Pending"
	ImageScanRequestPhaseInProgress ImageScanRequestPhase = "InProgress"
	ImageScanRequestPhaseCurrent    ImageScanRequestPhase = "Current"
	ImageScanRequestPhaseFailed     ImageScanRequestPhase = "Failed"
	ImageScanRequestPhaseOutdated   ImageScanRequestPhase = "Outdated"
)

type ImageDetails struct {
	Name       string
	Visibility ImageVisibility
	Tag        string
	Digest     string
}

type ScanReportRef struct {
	Name        string
	LastChecked trivy.Time
}

// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ImageScanRequestList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []ImageScanRequest
}
