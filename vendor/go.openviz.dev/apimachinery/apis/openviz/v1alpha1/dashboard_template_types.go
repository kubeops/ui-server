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
)

const (
	ResourceKindGrafanaDashboardTemplate = "GrafanaDashboardTemplate"
	ResourceGrafanaDashboardTemplate     = "grafanadashboardtemplate"
	ResourceGrafanaDashboardTemplates    = "grafanadashboardtemplates"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=grafanadashboardtemplates,singular=grafanadashboardtemplate,scope=Cluster,categories={grafana,openviz,appscode}
// +kubebuilder:subresource:status
type GrafanaDashboardTemplate struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GrafanaDashboardTemplateSpec `json:"spec,omitempty"`
}

type GrafanaDashboardTemplateSpec struct {
	GrafanaDashboardTemplate GrafanaDashboardTemplateReference `json:"grafanadashboardtemplate"`
	FolderID                 int64                             `json:"folderID"`
	Overwrite                bool                              `json:"overwrite"`
}

type GrafanaDashboardTemplateReference struct {
	ID            *int64   `json:"id"`
	UID           *string  `json:"uid"`
	Title         string   `json:"title"`
	Tags          []string `json:"tags"`
	Timezone      string   `json:"timezone"`
	SchemaVersion int64    `json:"schemaVersion"`
	Version       int64    `json:"version"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

type GrafanaDashboardTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GrafanaDashboardTemplate `json:"items,omitempty"`
}
