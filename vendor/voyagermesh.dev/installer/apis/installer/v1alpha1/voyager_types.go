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
	"kmodules.xyz/resource-metadata/apis/shared"
)

const (
	ResourceKindVoyager = "Voyager"
	ResourceVoyager     = "voyager"
	ResourceVoyagers    = "voyagers"
)

// Voyager defines the schama for Voyager Operator Installer.

// +genclient
// +genclient:skipVerbs=updateStatus
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
type Voyager struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VoyagerSpec `json:"spec,omitempty"`
}

// VoyagerSpec is the schema for Operator Operator values file
type VoyagerSpec struct {
	//+optional
	NameOverride string `json:"nameOverride"`
	//+optional
	FullnameOverride string       `json:"fullnameOverride"`
	ReplicaCount     int32        `json:"replicaCount"`
	Operator         ContianerRef `json:"operator"`
	Haproxy          ImageRef     `json:"haproxy"`
	Cleaner          CleanerRef   `json:"cleaner"`
	ImagePullPolicy  string       `json:"imagePullPolicy"`
	//+optional
	ImagePullSecrets []string `json:"imagePullSecrets"`
	//+optional
	CloudProvider *string `json:"cloudProvider"`
	//+optional
	CloudConfig string `json:"cloudConfig"`
	//+optional
	CriticalAddon bool `json:"criticalAddon"`
	//+optional
	LogLevel    int32       `json:"logLevel"`
	Persistence CloudConfig `json:"persistence"`
	//+optional
	Annotations map[string]string `json:"annotations"`
	//+optional
	PodAnnotations map[string]string `json:"podAnnotations"`
	//+optional
	NodeSelector map[string]string `json:"nodeSelector"`
	// If specified, the pod's tolerations.
	// +optional
	Tolerations []core.Toleration `json:"tolerations"`
	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *core.Affinity `json:"affinity"`
	// PodSecurityContext holds pod-level security attributes and common container settings.
	// Optional: Defaults to empty.  See type description for default values of each field.
	// +optional
	PodSecurityContext *core.PodSecurityContext `json:"podSecurityContext"`
	ServiceAccount     ServiceAccountSpec       `json:"serviceAccount"`
	// +optional
	IngressClass *string     `json:"ingressClass"`
	Apiserver    WebHookSpec `json:"apiserver"`
	Templates    Templates   `json:"templates"`
	License      string      `json:"license"`
	RegistryFQDN string      `json:"registryFQDN"`
	// +optional
	Distro shared.DistroSpec `json:"distro"`
}

type ImageRef struct {
	// +optional
	Registry string `json:"registry"`
	// +optional
	Repository string `json:"repository"`
	// +optional
	Tag string `json:"tag"`
}

type CleanerRef struct {
	ImageRef `json:",inline"`
	Skip     bool `json:"skip"`
}

type ContianerRef struct {
	ImageRef `json:",inline"`
	// Compute Resources required by the sidecar container.
	// +optional
	Resources core.ResourceRequirements `json:"resources"`
	// Security options the pod should run with.
	// +optional
	SecurityContext *core.SecurityContext `json:"securityContext"`
}

type CloudConfig struct {
	//+optional
	Enabled  bool   `json:"enabled" protobuf:"varint,1,opt,name=enabled"`
	HostPath string `json:"hostPath" protobuf:"bytes,2,opt,name=hostPath"`
}

type ServiceAccountSpec struct {
	Create bool `json:"create"`
	//+optional
	Name *string `json:"name"`
	//+optional
	Annotations map[string]string `json:"annotations"`
}

type WebHookSpec struct {
	GroupPriorityMinimum int32  `json:"groupPriorityMinimum"`
	VersionPriority      int32  `json:"versionPriority"`
	CA                   string `json:"ca"`
	//+optional
	BypassValidatingWebhookXray bool            `json:"bypassValidatingWebhookXray"`
	UseKubeapiserverFqdnForAks  bool            `json:"useKubeapiserverFqdnForAks"`
	Healthcheck                 HealthcheckSpec `json:"healthcheck"`
	ServingCerts                ServingCerts    `json:"servingCerts"`
}

type HealthcheckSpec struct {
	//+optional
	Enabled bool `json:"enabled"`
}

type ServingCerts struct {
	Generate bool `json:"generate"`
	//+optional
	CaCrt string `json:"caCrt"`
	//+optional
	ServerCrt string `json:"serverCrt"`
	//+optional
	ServerKey string `json:"serverKey"`
}

type Templates struct {
	Cfgmap *string `json:"cfgmap"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VoyagerList is a list of Voyagers
type VoyagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of Voyager CRD objects
	Items []Voyager `json:"items,omitempty"`
}
