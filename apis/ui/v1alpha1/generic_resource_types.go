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
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metrics/api"
)

const (
	ResourceKindGenericResource = "GenericResource"
	ResourceGenericResource     = "genericresource"
	ResourceGenericResources    = "genericresources"
)

// GenericResource is the Schema for any resource supported by resource-metrics library

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GenericResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GenericResourceSpec   `json:"spec,omitempty"`
	Status GenericResourceStatus `json:"status,omitempty"`
}

type GenericResourceSpec struct {
	ClusterName          string                            `json:"clusterName,omitempty"`
	ClusterID            string                            `json:"clusterID,omitempty"`
	APIType              kmapi.ResourceID                  `json:"apiType"`
	Replicas             int64                             `json:"replicas,omitempty"`
	RoleReplicas         api.ReplicaList                   `json:"roleReplicas,omitempty"`
	Mode                 string                            `json:"mode,omitempty"`
	TotalResource        core.ResourceRequirements         `json:"totalResource"`
	AppResource          core.ResourceRequirements         `json:"appResource"`
	RoleResourceLimits   map[api.PodRole]core.ResourceList `json:"roleResourceLimits,omitempty"`
	RoleResourceRequests map[api.PodRole]core.ResourceList `json:"roleResourceRequests,omitempty"`
}

type GenericResourceStatus struct {
	// Status
	Status string `json:"status,omitempty"`
	// Message
	Message string `json:"message,omitempty"`
	// Conditions list of extracted conditions from Resource
	Conditions []Condition `json:"conditions,omitempty"`
}

// Condition defines the general format for conditions on Kubernetes resources.
// In practice, each kubernetes resource defines their own format for conditions, but
// most (maybe all) follows this structure.
type Condition struct {
	// Type condition type
	Type string `json:"type,omitempty"`
	// Status String that describes the condition status
	Status core.ConditionStatus `json:"status,omitempty"`
	// Reason one work CamelCase reason
	Reason string `json:"reason,omitempty"`
	// Message Human readable reason string
	Message string `json:"message,omitempty"`
}

// GenericResourceList contains a list of GenericResource

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GenericResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GenericResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GenericResource{}, &GenericResourceList{})
}
