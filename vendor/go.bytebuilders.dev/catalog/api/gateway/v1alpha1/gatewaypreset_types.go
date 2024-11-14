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
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	GatewayConfigKey       = "catalog.appscode.com/gateway-config"
	DefaultGatewayClassKey = "catalog.appscode.com/is-default-gatewayclass"
	DefaultConfigKey       = "catalog.appscode.com/is-default-gateway-config"
	DefaultPresetKey       = "catalog.appscode.com/is-default-gateway-preset"
)

// GatewayPresetSpec defines the desired state of GatewayPreset.
type GatewayPresetSpec struct {
	// +optional
	ParametersRef *gwv1.ParametersReference `json:"parametersRef,omitempty"`
}

// GatewayPresetStatus defines the observed state of GatewayPreset.
type GatewayPresetStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []kmapi.Condition `json:"conditions,omitempty"`

	// Specifies the current phase of the App
	// +optional
	Phase PresetPhase `json:"phase,omitempty"`

	// HelmRelease is the name of the helm release used to deploy this ui
	// The name format is typically <alias>-<db-name>
	// +optional
	HelmRelease *core.LocalObjectReference `json:"helmRelease,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;InProgress;Current;Failed
type PresetPhase string

const (
	PresetPhasePending    PresetPhase = "Pending"
	PresetPhaseInProgress PresetPhase = "InProgress"
	PresetPhaseCurrent    PresetPhase = "Current"
	PresetPhaseFailed     PresetPhase = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GatewayPreset is the Schema for the gatewaypresets API.
type GatewayPreset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayPresetSpec   `json:"spec,omitempty"`
	Status GatewayPresetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayPresetList contains a list of GatewayPreset.
type GatewayPresetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayPreset `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayPreset{}, &GatewayPresetList{})
}
