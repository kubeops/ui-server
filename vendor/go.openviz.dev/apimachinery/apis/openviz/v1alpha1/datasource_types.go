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
	ResourceKindGrafanaDatasource = "GrafanaDatasource"
	ResourceGrafanaDatasource     = "grafanadatasource"
	ResourceGrafanaDatasources    = "grafanadatasources"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=grafanadatasources,singular=grafanadatasource,categories={grafana,openviz,appscode}
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
type GrafanaDatasource struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GrafanaDatasourceSpec   `json:"spec,omitempty"`
	Status            GrafanaDatasourceStatus `json:"status,omitempty"`
}

type GrafanaDatasourceSpec struct {
	GrafanaRef        *kmapi.ObjectReference      `json:"grafanaRef"`
	ID                int64                       `json:"id,omitempty"`
	OrgID             int64                       `json:"orgId"`
	Name              string                      `json:"name"`
	Type              GrafanaDatasourceType       `json:"type"`
	Access            GrafanaDatasourceAccessType `json:"access"`
	URL               string                      `json:"url"`
	Password          string                      `json:"password,omitempty"`
	User              string                      `json:"user,omitempty"`
	Database          string                      `json:"database,omitempty"`
	BasicAuth         bool                        `json:"basicAuth,omitempty"`
	BasicAuthUser     string                      `json:"basicAuthUser,omitempty"`
	BasicAuthPassword string                      `json:"basicAuthPassword,omitempty"`
	IsDefault         bool                        `json:"isDefault,omitempty"`
	Editable          bool                        `json:"editable,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type GrafanaDatasourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GrafanaDatasource `json:"items,omitempty"`
}

type GrafanaDatasourceStatus struct {
	// ObservedGeneration is the most recent generation observed for this resource. It corresponds to the
	// resource's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration  int64             `json:"observedGeneration,omitempty"`
	GrafanaDatasourceID *int64            `json:"grafanadatasourceId,omitempty"`
	Phase               GrafanaPhase      `json:"phase,omitempty"`
	Reason              string            `json:"reason,omitempty"`
	Conditions          []kmapi.Condition `json:"conditions,omitempty"`
}
