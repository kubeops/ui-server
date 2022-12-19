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

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceKindImageScanRequest = "ImageScanRequest"
	ResourceImageScanRequest     = "imagescanrequest"
	ResourceImageScanRequests    = "imagescanrequests"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ImageScanRequest struct {
	metav1.TypeMeta `json:",inline"`
	// Name will be formed by hashing the ImageRef + Tag + Digest
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec describes the attributes for the Image Scan SingleReport
	Spec ImageScanRequestSpec `json:"spec,omitempty"`

	// Status holds all the SingleReport-related details of the specified image
	Status ImageScanRequestStatus `json:"status,omitempty"`
}

type ImageScanRequestSpec struct {
	Image string `json:"image"`
	// If some private image is referred in Image, this field will contain the ImagePullSecrets from the pod template.
	// +optional
	PullSecrets []core.LocalObjectReference `json:"pullSecrets,omitempty"`
	// Namespace tells where to look for the image pull secrets.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

type ImageScanRequestStatus struct {
	// observedGeneration is the most recent generation observed for this resource. It corresponds to the
	// resource's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Specifies the current phase of the database
	// +optional
	// +kubebuilder:default="Pending"
	Phase     ImageScanRequestPhase `json:"phase,omitempty"`
	Image     *ImageDetails         `json:"image,omitempty"`
	ReportRef *ScanReportRef        `json:"reportRef,omitempty"`

	// A brief CamelCase message indicating details about why the request is in this state.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// +kubebuilder:validation:Enum=Public;Private
type ImageVisibility string

const (
	ImagePublic  ImageVisibility = "Public"
	ImagePrivate ImageVisibility = "Private"
)

// +kubebuilder:validation:Enum=Pending;InProgress;Current;Failed;Outdated
type ImageScanRequestPhase string

const (
	ImageScanRequestPhasePending    ImageScanRequestPhase = "Pending"
	ImageScanRequestPhaseInProgress ImageScanRequestPhase = "InProgress"
	ImageScanRequestPhaseCurrent    ImageScanRequestPhase = "Current"
	ImageScanRequestPhaseFailed     ImageScanRequestPhase = "Failed"
	ImageScanRequestPhaseOutdated   ImageScanRequestPhase = "Outdated"
)

type ImageDetails struct {
	Name string `json:"name,omitempty"`
	// +kubebuilder:default="Public"
	Visibility ImageVisibility `json:"visibility,omitempty"`
	// Tag & Digest is optional field. One of these fields may not present
	// +optional
	Tag string `json:"tag,omitempty"`
	// +optional
	Digest string `json:"digest,omitempty"`
}

type ScanReportRef struct {
	Name string `json:"name"`
	// When the referred image was checked for the last time
	// +optional
	LastChecked trivy.Time `json:"lastChecked,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ImageScanRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of Memcached TPR objects
	Items []ImageScanRequest `json:"items,omitempty"`
}
