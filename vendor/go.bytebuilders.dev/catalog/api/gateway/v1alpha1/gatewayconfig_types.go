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
	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	dnsapi "kubeops.dev/external-dns-operator/apis/external/v1alpha1"
	voyagerinstaller "voyagermesh.dev/installer/apis/installer/v1alpha1"
)

// GatewayConfigSpec defines the desired state of GatewayConfig.
type GatewayConfigSpec struct {
	GatewaySpec `json:",inline"`
	Envoy       EnvoySpec `json:"envoy"`
	// Chart specifies the chart information that will be used by the FluxCD to install the respective feature
	// +optional
	Chart uiapi.ChartInfo `json:"chart,omitempty"`
}

type GatewaySpec struct {
	Infra      ServiceProviderInfra                `json:"infra"`
	Gateway    voyagerinstaller.VoyagerGatewaySpec `json:"gateway"`
	GatewayDns ServiceGatewayDns                   `json:"gateway-dns"`
	Cluster    ServiceProviderCluster              `json:"cluster"`
	Echoserver EchoserverSpec                      `json:"echoserver"`
	// +optional
	VaultServer kmapi.ObjectReference `json:"vaultServer"`
}

type GatewayValues struct {
	GatewaySpec `json:",inline"`
	Envoy       EnvoyValues `json:"envoy"`
}

type GatewayParameter struct {
	GatewayClassName     string                `json:"-"`
	ServiceType          egv1a1.ServiceType    `json:"-"`
	Service              EnvoyServiceSpec      `json:"service"`
	VaultServer          kmapi.ObjectReference `json:"vaultServer"`
	FrontendTLSSecretRef kmapi.ObjectReference `json:"frontendTLSSecretRef"`
}

type ServiceProviderInfra struct {
	HostInfo `json:",inline"`
	TLS      InfraTLS   `json:"tls"`
	DNS      GatewayDns `json:"dns"`
}

// +kubebuilder:validation:Enum=ca;letsencrypt;letsencrypt-staging;external
type TLSIssuerType string

const (
	TLSIssuerTypeCA        TLSIssuerType = "ca"
	TLSIssuerTypeLE        TLSIssuerType = "letsencrypt"
	TLSIssuerTypeLEStaging TLSIssuerType = "letsencrypt-staging"
	TLSIssuerTypeExternal  TLSIssuerType = "external"
)

type InfraTLS struct {
	Issuer      TLSIssuerType  `json:"issuer"`
	CA          *TLSData       `json:"ca,omitempty"`
	Acme        *TLSIssuerAcme `json:"acme,omitempty"`
	Certificate *TLSData       `json:"certificate,omitempty"`
	JKS         *Keystore      `json:"jks,omitempty"`
}

type TLSData struct {
	// +optional
	Cert string `json:"cert"`
	// +optional
	Key string `json:"key"`
}

type Keystore struct {
	// +optional
	Truststore []byte `json:"truststore"`
	// +optional
	Keystore []byte `json:"keystore"`
	Password string `json:"password"`
}

type TLSIssuerAcme struct {
	Email  string     `json:"email"`
	Solver AcmeSolver `json:"solver"`
}

// +kubebuilder:validation:Enum=Gateway;Ingress
type AcmeSolver string

const (
	AcmeSolverGateway = "Gateway"
	AcmeSolverIngress = "Ingress"
)

type HostInfo struct {
	Host     string   `json:"host"`
	HostType HostType `json:"hostType"`
}

// +kubebuilder:validation:Enum=domain;ip
// +kubebuilder:default=ip
type HostType string

const (
	HostTypeDomain HostType = "domain"
	HostTypeIP     HostType = "ip"
)

type GatewayDns struct {
	Provider DNSProvider     `json:"provider"`
	Auth     DNSProviderAuth `json:"auth"`
}

// +kubebuilder:validation:Enum=none;external;cloudflare;route53;cloudDNS;azureDNS
type DNSProvider string

const (
	DNSProviderNone       DNSProvider = "none"
	DNSProviderExternal   DNSProvider = "external"
	DNSProviderCloudflare DNSProvider = "cloudflare"
	DNSProviderRoute53    DNSProvider = "route53"
	DNSProviderCloudDNS   DNSProvider = "cloudDNS"
	DNSProviderAzureDNS   DNSProvider = "azureDNS"
)

type DNSProviderAuth struct {
	// WARNING!!! Update docs in schema/ace-options/patch.yaml
	Cloudflare *CloudflareAuth `json:"cloudflare,omitempty"`

	// WARNING!!! Update docs in schema/ace-options/patch.yaml
	Route53 *Route53Auth `json:"route53,omitempty"`

	// WARNING!!! Update docs in schema/ace-options/patch.yaml
	CloudDNS *CloudDNSAuth `json:"cloudDNS,omitempty"`

	// WARNING!!! Update docs in schema/ace-options/patch.yaml
	AzureDNS *AzureDNSAuth `json:"azureDNS,omitempty"`
}

type CloudflareAuth struct {
	// +optional
	BaseURL string `json:"baseURL,omitempty"`
	Token   string `json:"token"`
}

type Route53Auth struct {
	AwsAccessKeyID     string `json:"AWS_ACCESS_KEY_ID"`
	AwsSecretAccessKey string `json:"AWS_SECRET_ACCESS_KEY"`
	AwsRegion          string `json:"AWS_REGION"`
}

type CloudDNSAuth struct {
	GoogleProjectID             string `json:"GOOGLE_PROJECT_ID"`
	GoogleServiceAccountJSONKey string `json:"GOOGLE_SERVICE_ACCOUNT_JSON_KEY"`
}

type AzureDNSAuth struct {
	SubscriptionID              string `json:"subscriptionID"`
	TenantID                    string `json:"tenantID"`
	ResourceGroupName           string `json:"resourceGroupName"`
	HostedZoneName              string `json:"hostedZoneName"`
	ServicePrincipalAppID       string `json:"servicePrincipalAppID"`
	ServicePrincipalAppPassword string `json:"servicePrincipalAppPassword"`
	// +optional
	Environment string `json:"environment,omitempty"`
}

type ServiceGatewayDns struct {
	Enabled bool                    `json:"enabled"`
	Spec    *dnsapi.ExternalDNSSpec `json:"spec,omitempty"`
}

type ServiceProviderCluster struct {
	TLS ClusterTLS `json:"tls"`
}

type ClusterTLS struct {
	Issuer ClusterTLSIssuerType `json:"issuer"`
	CA     TLSData              `json:"ca"`
}

type EnvoySpec struct {
	Image string `json:"image"`
	Tag   string `json:"tag"`
	//+optional
	SecurityContext *core.SecurityContext `json:"securityContext"`
	Service         EnvoyServiceSpec      `json:"service"`
}

type EnvoyValues struct {
	Image string `json:"image"`
	Tag   string `json:"tag"`
	//+optional
	SecurityContext *core.SecurityContext `json:"securityContext"`
	Service         EnvoyServiceValues    `json:"service"`
}

const (
	AllocatedPortsKey    = "catalog.appscode.com/allocated-ports"
	SeedPortKey          = "catalog.appscode.com/seed-port"
	DefaultPortRange     = "10000-12767"
	DefaultNodeportRange = "30000-32767"
)

type EnvoyServiceSpec struct {
	// +kubebuilder:default="10000-12767"
	PortRange string `json:"portRange"`
	// +kubebuilder:default="30000-32767"
	NodeportRange string `json:"nodeportRange"`

	// +kubebuilder:default=LoadBalancer
	Type egv1a1.ServiceType `json:"type"`
	// +kubebuilder:default=Cluster
	ExternalTrafficPolicy egv1a1.ServiceExternalTrafficPolicy `json:"externalTrafficPolicy"`
	ExternalIPs           []string                            `json:"externalIPs,omitempty"`
}

type EnvoyServiceValues struct {
	EnvoyServiceSpec `json:",inline"`
	// +kubebuilder:default=8080
	SeedBackendPort int32 `json:"seedBackendPort"`
}

type EchoserverSpec struct {
	Image string `json:"image"`
	Tag   string `json:"tag"`
	//+optional
	SecurityContext *core.SecurityContext `json:"securityContext"`
}

// +kubebuilder:validation:Enum=ca
type ClusterTLSIssuerType string

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GatewayConfig is the Schema for the gatewayconfigs API.
type GatewayConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GatewayConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayConfigList contains a list of GatewayConfig.
type GatewayConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayConfig{}, &GatewayConfigList{})
}
