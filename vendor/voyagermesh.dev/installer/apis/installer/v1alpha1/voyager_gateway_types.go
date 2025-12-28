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
	autoscaling "k8s.io/api/autoscaling/v2"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ResourceKindVoyagerGateway = "VoyagerGateway"
	ResourceVoyagerGateway     = "oyagergateway"
	ResourceVoyagerGateways    = "oyagergateways"
)

// VoyagerGateway defines the schama for VoyagerGateway operator installer.

// +genclient
// +genclient:skipVerbs=updateStatus
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
type VoyagerGateway struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VoyagerGatewaySpec `json:"spec,omitempty"`
}

// VoyagerGatewaySpec is the schema for Operator Operator values file
type VoyagerGatewaySpec struct {
	Global                  *VoyagerGatewayGlobal    `json:"global,omitempty"`
	PodDisruptionBudget     *PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	Deployment              *DeploymentSpec          `json:"deployment,omitempty"`
	Service                 *ServiceSpec             `json:"service"`
	Hpa                     *HPASpec                 `json:"hpa"`
	Config                  *EnvoyGatewayConfig      `json:"config,omitempty"`
	CreateNamespace         *bool                    `json:"createNamespace,omitempty"`
	KubernetesClusterDomain *string                  `json:"kubernetesClusterDomain,omitempty"`
	Certgen                 *CertgenSpec             `json:"certgen,omitempty"`
	TopologyInjector        *TopologyInjectorSpec    `json:"topologyInjector"`
	GatewayConverter        *VoyagerGatewayConverter `json:"gateway-converter,omitempty"`
}

type VoyagerGatewayGlobal struct {
	ImageRegistry    string   `json:"imageRegistry"`
	ImagePullSecrets []string `json:"imagePullSecrets"`
	Images           Images   `json:"images"`
}

type Images struct {
	EnvoyGateway ImageDetails `json:"envoyGateway"`
	Ratelimit    ImageDetails `json:"ratelimit"`
}

type ImageDetails struct {
	Image       string                      `json:"image"`
	PullPolicy  string                      `json:"pullPolicy"`
	PullSecrets []core.LocalObjectReference `json:"pullSecrets"`
}

type PodDisruptionBudgetSpec struct {
	MinAvailable int `json:"minAvailable"`
}

type DeploymentSpec struct {
	EnvoyGateway      *EnvoyGatewayDeployment `json:"envoyGateway,omitempty"`
	Ports             []Port                  `json:"ports,omitempty"`
	PriorityClassName *string                 `json:"priorityClassName"`
	Replicas          *int                    `json:"replicas,omitempty"`
	Pod               *PodTemplateSpec        `json:"pod,omitempty"`
}

type ServiceSpec struct {
	TrafficDistribution string            `json:"trafficDistribution"`
	Annotations         map[string]string `json:"annotations"`
}
type HPASpec struct {
	Enabled     bool                                         `json:"enabled"`
	MinReplicas int                                          `json:"minReplicas"`
	MaxReplicas int                                          `json:"maxReplicas"`
	Metrics     []autoscaling.MetricSpec                     `json:"metrics"`
	Behavior    *autoscaling.HorizontalPodAutoscalerBehavior `json:"behavior"`
}
type EnvoyGatewayDeployment struct {
	Image           *ImageSpec `json:"image,omitempty"`
	ImagePullPolicy *string    `json:"imagePullPolicy,omitempty"`
	// +optional
	ImagePullSecrets []core.LocalObjectReference `json:"imagePullSecrets"`
	Resources        *core.ResourceRequirements  `json:"resources,omitempty"`
	SecurityContext  *core.SecurityContext       `json:"securityContext,omitempty"`
}

type ImageSpec struct {
	Repository *string `json:"repository,omitempty"`
	Tag        *string `json:"tag,omitempty"`
}

type Port struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	TargetPort int    `json:"targetPort"`
}

type PodTemplateSpec struct {
	// +optional
	Affinity *core.Affinity `json:"affinity"`
	// +optional
	Annotations map[string]string `json:"annotations"`
	// +optional
	Labels map[string]string `json:"labels"`
	// +optional
	TopologySpreadConstraints []core.TopologySpreadConstraint `json:"topologySpreadConstraints"`
	// +optional
	Tolerations []core.Toleration `json:"tolerations"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector"`
}

type EnvoyGatewayConfig struct {
	EnvoyGateway EnvoyGatewaySpec `json:"envoyGateway"`
}

type EnvoyGatewaySpec struct {
	Gateway       *GatewayControllerSpec `json:"gateway,omitempty"`
	Provider      *GatewayProviderSpec   `json:"provider,omitempty"`
	Logging       *LoggingSpec           `json:"logging,omitempty"`
	ExtensionApis runtime.RawExtension   `json:"extensionApis"`
}

type GatewayControllerSpec struct {
	ControllerName string `json:"controllerName"`
}

type GatewayProviderSpec struct {
	Type string `json:"type"`
}

type LoggingSpec struct {
	Level LoggingLevel `json:"level"`
}

type LoggingLevel struct {
	Default string `json:"default"`
}

type CertgenSpec struct {
	Job CertgenJobSpec `json:"job"`
	// +optional
	Rbac CertgenRbacMetadata `json:"rbac"`
}

type CertgenJobSpec struct {
	// +optional
	Annotations map[string]string `json:"annotations"`
	// +optional
	Resources core.ResourceRequirements `json:"resources"`
	// +optional
	TtlSecondsAfterFinished int                   `json:"ttlSecondsAfterFinished"`
	SecurityContext         *core.SecurityContext `json:"securityContext,omitempty"`
	// +optional
	Affinity *core.Affinity `json:"affinity"`
	// +optional
	Args []string `json:"args"`
	// +optional
	Tolerations []core.Toleration `json:"tolerations"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector"`
}

type CertgenRbacMetadata struct {
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`
}

type TopologyInjectorSpec struct {
	Enabled     bool              `json:"enabled"`
	Annotations map[string]string `json:"annotations"`
}

type VoyagerGatewayConverter struct {
	Enabled               bool `json:"enabled"`
	*GatewayConverterSpec `json:",inline,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VoyagerGatewayList is a list of VoyagerGateways
type VoyagerGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of VoyagerGateway CRD objects
	Items []VoyagerGateway `json:"items,omitempty"`
}
