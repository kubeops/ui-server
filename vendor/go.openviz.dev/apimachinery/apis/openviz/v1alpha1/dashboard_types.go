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
	"k8s.io/apimachinery/pkg/runtime"
	kmapi "kmodules.xyz/client-go/api/v1"
)

const (
	ResourceKindGrafanaDashboard = "GrafanaDashboard"
	ResourceGrafanaDashboard     = "grafanadashboard"
	ResourceGrafanaDashboards    = "grafanadashboards"
)

const (
	GrafanaDashboardTitleKey = ".dashboard.title"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=grafanadashboards,singular=grafanadashboard,categories={grafana,openviz,appscode}
// +kubebuilder:printcolumn:name="Title",type="string",JSONPath=".spec.model.title"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type GrafanaDashboard struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GrafanaDashboardSpec   `json:"spec,omitempty"`
	Status            GrafanaDashboardStatus `json:"status,omitempty"`
}

type GrafanaDashboardSpec struct {
	// GrafanaRef defines the grafana app binding name for the GrafanaDashboard
	// +optional
	GrafanaRef *kmapi.ObjectReference `json:"grafanaRef,omitempty"`

	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Model *runtime.RawExtension `json:"model,omitempty"`

	// Overwrite defines the existing grafanadashboard with the same name(if any) should be overwritten or not
	// +optional
	Overwrite bool `json:"overwrite,omitempty"`

	// Templatize defines the fields which supports templating in GrafanaDashboard Model json
	// +optional
	Templatize *ModelTemplateConfiguration `json:"templatize,omitempty"`
}

type ModelTemplateConfiguration struct {
	Title      bool `json:"title,omitempty"`
	Datasource bool `json:"datasource,omitempty"`
}

type GrafanaDashboardReference struct {
	ID      *int64  `json:"id,omitempty"`
	UID     *string `json:"uid,omitempty"`
	OrgID   *int64  `json:"orgID,omitempty"`
	Slug    *string `json:"slug,omitempty"`
	URL     *string `json:"url,omitempty"`
	Version *int64  `json:"version,omitempty"`
	State   *string `json:"state,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type GrafanaDashboardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GrafanaDashboard `json:"items,omitempty"`
}

type GrafanaDashboardStatus struct {
	// ObservedGeneration is the most recent generation observed for this resource. It corresponds to the
	// resource's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase indicates the state this Vault cluster jumps in.
	// +optional
	Phase GrafanaPhase `json:"phase,omitempty"`

	// The reason for the current phase
	// +optional
	Reason string `json:"reason,omitempty"`

	// Dashboard indicates the updated grafanadashboard database
	// +optional
	Dashboard *GrafanaDashboardReference `json:"dashboard,omitempty"`

	// Represents the latest available observations of a GrafanaDashboard current state.
	// +optional
	Conditions []kmapi.Condition `json:"conditions,omitempty"`
}
