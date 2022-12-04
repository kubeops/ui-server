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
	Images []ImageReport `json:"images"`
	// Aggregated for all the images under this ref. ex HIGH: 3, MEDIUM: 7, LOW: 4
	Vulnerabilities map[string]int `json:"vulnerabilities,omitempty"`
}

type ImageReport struct {
	Name               string                    `json:"name"` // Name + (Tag if any)
	Digest             string                    `json:"digest"`
	VulnerabilityInfos []VulnerabilityInfo       `json:"vulnerabilityInfos,omitempty"`
	Pods               []string                  `json:"pods"`
	Containers         []string                  `json:"containers"`
	Lineage            [][]metav1.OwnerReference `json:"lineage,omitempty"`
}

type VulnerabilityInfo struct {
	PkgName          string `json:"PkgName"`
	InstalledVersion string `json:"InstalledVersion"`
	Title            string `json:"Title"`
	Severity         string `json:"Severity"`
	URL              string `json:"URL"`
	FixedVersion     string `json:"FixedVersion"`
}
