/*
Copyright AppsCode Inc. and Contributors

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
	ResourceKindDashboardGroup = "DashboardGroup"
	ResourceDashboardGroup     = "dashboardgroup"
	ResourceDashboardGroups    = "dashboardgroups"
)

// DashboardGroup is the Schema for the DashboardGroups API

// +genclient
// +genclient:nonNamespaced
// +genclient:onlyVerbs=create
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type DashboardGroup struct {
	metav1.TypeMeta `json:",inline"`

	Request  *DashboardGroupRequest  `json:"request,omitempty"`
	Response *DashboardGroupResponse `json:"response,omitempty"`
}

type DashboardGroupRequest struct {
	Dashboards []DashboardRequest `json:"dashboards"`
	// +optional
	// +kubebuilder:default="30s"
	RefreshInterval string `json:"refreshInterval,omitempty"`
	// +optional
	//
	// +kubebuilder:default={from: "now-3h", to: "now"}
	TimeRange *TimeRange `json:"timeRange,omitempty"`
	// +optional
	EmbeddedLink bool `json:"embeddedLink,omitempty"`
}

type TimeRange struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type DashboardRequest struct {
	DashboardRef `json:",inline"`
	// +optional
	Vars []DashboardVar `json:"vars,omitempty"`
	// +optional
	Panels []PanelLinkRequest `json:"panels,omitempty"`
}

type PanelLinkRequest struct {
	Title string `json:"title"`
	// +optional
	Width int `json:"width,omitempty"`
}

type DashboardRef struct {
	// +optional
	*kmapi.ObjectReference `json:",inline"`
	// +optional
	Title string `json:"title,omitempty"`
}

// +kubebuilder:validation:Enum=Source;Target
type DashboardVarType string

const (
	DashboardVarTypeSource DashboardVarType = "Source"
	DashboardVarTypeTarget DashboardVarType = "Target"
)

type DashboardVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// +optional
	// +kubebuilder:default:=Source
	Type DashboardVarType `json:"type,omitempty"`
}

type DashboardGroupResponse struct {
	Dashboards []DashboardResponse `json:"dashboards"`
}

type DashboardResponse struct {
	DashboardRef `json:",inline"`
	// +optional
	URL string `json:"url,omitempty"`
	// +optional
	Panels []PanelLinkResponse `json:"panels,omitempty"`
}

type PanelLinkResponse struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	// +optional
	Width int `json:"width,omitempty"`
}
