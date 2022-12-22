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
	kmapi "kmodules.xyz/client-go/api/v1"
)

const (
	ResourceKindCVEReport = "CVEReport"
	ResourceCVEReport     = "cvereport"
	ResourceCVEReports    = "cvereports"
)

// +genclient
// +genclient:nonNamespaced
// +genclient:onlyVerbs=create
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CVEReport struct {
	metav1.TypeMeta `json:",inline"`
	// 1. Request equals nil means, we want the report in cluster scope
	// 2. Group is set to ""(core) & Kind to Namespace means, we want the report in particular namespaced scope
	// 3. For general cases, all the fields need to be set.
	// +optional
	Request *CVEReportRequest `json:"request,omitempty"`
	// +optional
	Response *CVEReportResponse `json:"response,omitempty"`
}

type CVEReportRequest struct {
	kmapi.ObjectInfo `json:",inline"`
}

type CVEReportResponse struct {
	Images          []ImageInfo       `json:"images"`
	Vulnerabilities VulnerabilityInfo `json:"vulnerabilities"`
}

type VulnerabilityInfo struct {
	Stats map[string]RiskStats  `json:"stats"`
	CVEs  []trivy.Vulnerability `json:"cves"`
}

type RiskStats struct {
	Count      int `json:"count"`
	Occurrence int `json:"occurrence"`
}

type ImageInfo struct {
	Image      ImageReference  `json:"image"`
	Metadata   *ImageMetadata  `json:"metadata,omitempty"`
	Lineages   []kmapi.Lineage `json:"lineages,omitempty"`
	ScanStatus ImageScanStatus `json:"scanStatus"`
}

type ScanResult string

const (
	ScanResultPending  ScanResult = "Pending"
	ScanResultFound    ScanResult = "Found"
	ScanResultNotFound ScanResult = "NotFound"
	ScanResultError    ScanResult = "Error"
)

type ImageScanStatus struct {
	Result ScanResult `json:"result"`
	// A human-readable message indicating details about scan result.
	// +optional
	Message string `json:"message,omitempty"`

	ReportRef *core.LocalObjectReference `json:"reportRef,omitempty"`

	// When the referred image was checked for the last time
	// +optional
	LastChecked *trivy.Time `json:"lastChecked,omitempty"`

	// which TrivyDBVersion was used when the last check
	// +optional
	TrivyDBVersion string `json:"trivyDBVersion,omitempty"`
}

type ImageReference struct {
	Name   string `json:"name"`
	Tag    string `json:"tag,omitempty"`
	Digest string `json:"digest,omitempty"`
}

type ImageMetadata struct {
	Os          *trivy.ImageOS `json:"os,omitempty"`
	ImageConfig *ImageConfig   `json:"imageConfig,omitempty"`
}

type ImageConfig struct {
	Architecture string `json:"architecture,omitempty"`
	Author       string `json:"author,omitempty"`
	Container    string `json:"container,omitempty"`
	Os           string `json:"os,omitempty"`
}
