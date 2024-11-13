/*
Copyright AppsCode Inc. and Contributors

Licensed under the AppsCode Community License 1.0.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://github.com/appscode/licenses/raw/1.0.0/AppsCode-Community-1.0.0.md

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
)

const (
	ResourceKindGatewayConverter = "GatewayConverter"
	ResourceGatewayConverter     = "gatewayconverter"
	ResourceGatewayConverters    = "gatewayconverters"
)

// GatewayConverter defines the schama for GatewayConverter operator installer.

// +genclient
// +genclient:skipVerbs=updateStatus
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
type GatewayConverter struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GatewayConverterSpec `json:"spec,omitempty"`
}

// GatewayConverterSpec is the schema for Operator Operator values file
type GatewayConverterSpec struct {
	//+optional
	NameOverride *string `json:"nameOverride,omitempty"`
	//+optional
	FullnameOverride *string    `json:"fullnameOverride,omitempty"`
	RegistryFQDN     *string    `json:"registryFQDN,omitempty"`
	ReplicaCount     *int32     `json:"replicaCount,omitempty"`
	Server           *Container `json:"server,omitempty"`
	ImagePullPolicy  *string    `json:"imagePullPolicy,omitempty"`
	//+optional
	ImagePullSecrets []string `json:"imagePullSecrets"`
	//+optional
	CriticalAddon bool `json:"criticalAddon"`
	//+optional
	LogLevel int32 `json:"logLevel,omitempty"`
	//+optional
	Annotations map[string]string `json:"annotations"`
	//+optional
	PodAnnotations map[string]string `json:"podAnnotations"`
	//+optional
	PodLabels map[string]string `json:"podLabels"`
	//+optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// If specified, the pod's tolerations.
	// +optional
	Tolerations []core.Toleration `json:"tolerations"`
	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *core.Affinity `json:"affinity,omitempty"`
	// PodSecurityContext holds pod-level security attributes and common container settings.
	// Optional: Defaults to empty.  See type description for default values of each field.
	// +optional
	PodSecurityContext *core.PodSecurityContext   `json:"podSecurityContext,omitempty"`
	ServiceAccount     *ServiceAccountSpec        `json:"serviceAccount,omitempty"`
	HostNetwork        *bool                      `json:"hostNetwork,omitempty"`
	Apiserver          *GatewayConverterApiserver `json:"apiserver,omitempty"`
	Monitoring         *Monitoring                `json:"monitoring,omitempty"`
}

type Container struct {
	ImageRef `json:",inline"`
	// Compute Resources required by the sidecar container.
	// +optional
	Resources core.ResourceRequirements `json:"resources"`
	// Security options the pod should run with.
	// +optional
	SecurityContext *core.SecurityContext `json:"securityContext"`
}

type GatewayConverterApiserver struct {
	Healthcheck  HealthcheckSpec `json:"healthcheck"`
	ServingCerts ServingCerts    `json:"servingCerts"`
}

// +kubebuilder:validation:Enum=prometheus.io;prometheus.io/operator;prometheus.io/builtin
type MonitoringAgent string

type Monitoring struct {
	Agent          MonitoringAgent      `json:"agent"`
	ServiceMonitor ServiceMonitorLabels `json:"serviceMonitor"`
}

type ServiceMonitorLabels struct {
	// +optional
	Labels map[string]string `json:"labels"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GatewayConverterList is a list of GatewayConverters
type GatewayConverterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of GatewayConverter CRD objects
	Items []GatewayConverter `json:"items,omitempty"`
}
