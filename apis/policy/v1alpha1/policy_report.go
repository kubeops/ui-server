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
	"github.com/open-policy-agent/gatekeeper/pkg/audit"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
)

const (
	ResourceKindPolicyReport = "PolicyReport"
	ResourcePolicyReport     = "policyreport"
	ResourcePolicyReports    = "policyreports"
)

// +genclient
// +genclient:nonNamespaced
// +genclient:onlyVerbs=create
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PolicyReport struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	Request *PolicyReportRequest `json:"request,omitempty"`
	// +optional
	Response *PolicyReportResponse `json:"response,omitempty"`
}

type PolicyReportRequest struct {
	kmapi.ObjectInfo `json:",inline"`
}

type PolicyReportResponse struct {
	Constraints []Constraint `json:"constraints,omitempty"`
}

type Constraint struct {
	AuditTimestamp metav1.Time             `json:"auditTimestamp,omitempty"`
	Name           string                  `json:"name,omitempty"`
	Violations     []audit.StatusViolation `json:"violations,omitempty"`
}
