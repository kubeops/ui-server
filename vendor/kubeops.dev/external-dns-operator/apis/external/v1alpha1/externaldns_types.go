/*
Copyright 2022.

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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kmapi "kmodules.xyz/client-go/api/v1"
)

// TypeInfo is for source type, contains the group,version,kind information of the source
type TypeInfo struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

func (t TypeInfo) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   t.Group,
		Version: t.Version,
		Kind:    t.Kind,
	}
}

type AWSProvider struct {
	// When using the AWS provider, filter for zones of this type. (support: public, private)
	// +optional
	ZoneType *string `json:"zoneType,omitempty"`

	// When using the AWS provider, filter for zones with these tags
	// +optional
	ZoneTagFilter []string `json:"zoneTagFilter,omitempty"`

	// When using the AWS provider, assume this IAM role. Useful for hosted zones in another AWS account. Specify the
	// full ARN, e.g. `arn:aws:iam::123455567:role/external-dns`
	// +optional
	AssumeRole *string `json:"assumeRole,omitempty"`

	// When using AWS provide, set the maximum number of changes that will be applied in each batch
	// +optional
	BatchChangeSize *int `json:"batchChangeSize,omitempty"`

	// When using the AWS provider, set the interval between batch changes.
	// +optional
	BatchChangeInterval *time.Duration `json:"batchChangeInterval,omitempty"`

	// When using the AWS provider, set whether to evaluate the health of the DNS target (default: enable, disable with --no-aws-evaluate-target-health)
	// +optional
	EvaluateTargetHealth *bool `json:"evaluateTargetHealth,omitempty"`

	// When using the AWS provider, set the maximum number of retries for API calls before giving up.
	// +optional
	APIRetries *int `json:"apiRetries,omitempty"`

	// When using the AWS provider, prefer using CNAME instead of ALIAS (default: disabled)
	// +optional
	PreferCNAME *bool `json:"preferCNAME,omitempty"`

	// When using the AWS provider, set the zones list cache TTL (0s to disable).
	// +optional
	ZoneCacheDuration *time.Duration `json:"zoneCacheDuration,omitempty"`

	// When using the AWS CloudMap provider, delete empty Services without endpoints (default: disabled)
	// +optional
	SDServiceCleanup *bool `json:"sdServiceCleanup,omitempty"`

	SDCreateTag *map[string]string `json:"sdCreateTag"`

	// provider secret credential information
	// +optional
	SecretRef *GenericSecretReference `json:"secretRef,omitempty"`
}

type CloudflareProvider struct {
	// When using the Cloudflare provider, specify if the proxy mode must be enabled (default: disabled)
	// +optional
	Proxied                             *bool   `json:"proxied,omitempty"`
	CustomHostnames                     *bool   `json:"customHostnames,omitempty"` // new
	CustomHostnamesCertificateAuthority *string `json:"customHostnamesCertificateAuthority,omitempty"`
	CustomHostnamesMinTLSVersion        *string `json:"customHostnamesMinTLSVersion,omitempty"`
	RegionalServices                    *bool   `json:"regionalServices,omitempty"`
	RegionKey                           *string `json:"regionKey,omitempty"`

	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// +optional
	SecretRef *CloudflareSecretReference `json:"secretRef,omitempty"`
}

type AzureProvider struct {
	// When using the Azure provider, override the Azure resource group to use (required for azure-private-dns)
	// +optional
	ResourceGroup *string `json:"resourceGroup,omitempty"`

	// When using the Azure provider, specify the Azure configuration file. (required for azure-private-dns)
	// +optional
	SubscriptionId *string `json:"subscriptionId,omitempty"`

	// When using the Azure provider, override the client id of user assigned identity in config file
	// +optional
	UserAssignedIdentityClientID *string `json:"userAssignedIdentityClientID,omitempty"`

	ZonesCacheDuration *time.Duration `json:"zonesCacheDuration"` // new
	MaxRetriesCount    *int           `json:"maxRetriesCount"`

	// Provider secret credential information
	SecretRef *GenericSecretReference `json:"secretRef,omitempty"`
}

type GoogleProvider struct {
	// When using the Google provider, current project is auto-detected, when running on GCP. Specify other project with this. Must be specified when running outside GCP.
	// +optional
	Project *string `json:"project,omitempty"`

	// When using the Google provider, set the maximum number of changes that will be applied in each batch
	// +optional
	BatchChangeSize *int `json:"batchChangeSize,omitempty"`

	// When using the Google provider, set the interval between batch changes
	// +optional
	BatchChangeInterval *time.Duration `json:"batchChangeInterval,omitempty"`

	// When using the Google provider, filter for zones with this visibility (optional, options: public, private)
	// +optional
	ZoneVisibility *string `json:"zoneVisibility,omitempty"`

	// Provider secret credential information
	SecretRef *GenericSecretReference `json:"secretRef,omitempty"`
}

type ServiceConfig struct {
	// Limit sources of endpoints to a specific namespace (default: all namespaces)
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// Ignore hostname annotation when generating DNS names, valid only when using fqdn-template is set
	// +optional
	IgnoreHostnameAnnotation *bool `json:"ignoreHostnameAnnotation,omitempty"`

	// Combine FQDN template and Annotations instead of overwriting
	// +optional
	CombineFQDNAndAnnotation *bool `json:"combineFQDNAndAnnotation,omitempty"`

	// Filter sources managed by external-dns via label selector when listing all resources
	// +optional
	AnnotationFilter *string `json:"annotationFilter,omitempty"`

	// Filter sources managed by external-dns via annotation using label selector semantics
	// +optional
	LabelFilter *string `json:"labelFilter,omitempty"`

	// A templated string that's used to generate DNS names from source that don't define a hostname themselves, or to
	// add a hostname suffix when paired with the fake source
	// +optional
	FQDNTemplate *string `json:"fqdnTemplate,omitempty"`

	// Process  annotation semantics from legacy implementations
	// +optional
	Compatibility *string `json:"compatibility,omitempty"`

	// Allow  externals-dns to publish DNS records for ClusterIP services
	// +optional
	PublishInternal *bool `json:"publishInternal,omitempty"`

	// Allow external-dns to publish host-ip for headless services
	// +optional
	PublishHostIP *bool `json:"publishHostIP,omitempty"`

	// Always publish also not ready addresses for headless services
	// +optional
	AlwaysPublishNotReadyAddresses *bool `json:"alwaysPublishNotReadyAddresses,omitempty"`

	// The service types to take care about (default all, expected: ClusterIP, NodePort, LoadBalancer or ExternalName)
	// +optional
	ServiceTypeFilter []string `json:"serviceTypeFilter,omitempty"`
}

type IngressConfig struct {
	// Limit sources of endpoints to a specific namespace (default: all namespaces)
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// Ignore hostname annotation when generating DNS names, valid only when using fqdn-template is set
	// +optional
	IgnoreHostnameAnnotation *bool `json:"ignoreHostnameAnnotation,omitempty"`

	// Combine FQDN template and Annotations instead of overwriting
	// +optional
	CombineFQDNAndAnnotation *bool `json:"combineFQDNAndAnnotation,omitempty"`

	// Filter sources managed by external-dns via label selector when listing all resources
	// +optional
	AnnotationFilter *string `json:"annotationFilter,omitempty"`

	// Filter sources managed by external-dns via annotation using label selector semantics
	// +optional
	LabelFilter *string `json:"labelFilter,omitempty"`

	// A templated string that's used to generate DNS names from source that don't define a hostname themselves, or to
	// add a hostname suffix when paired with the fake source
	// +optional
	FQDNTemplate *string `json:"fqdnTemplate,omitempty"`

	// Ignore TLS Spec section in ingresses resources, applicable only for ingress source
	// +optional
	IgnoreIngressTLSSpec *bool `json:"ignoreIngressTLSSpec,omitempty"`

	// Ignore rules spec section in ingresses resources, applicable only for ingress sources
	// +optional
	IgnoreIngressRulesSpec *bool `json:"ignoreIngressRulesSpec,omitempty"`
}

type NodeConfig struct {
	// A templated string that's used to generate DNS names from source that don't define a hostname themselves, or to
	// add a hostname suffix when paired with the fake source
	FQDNTemplate string `json:"fqdnTemplate,omitempty"`

	// Filter sources managed by external-dns via label selector when listing all resources
	// +optional
	AnnotationFilter *string `json:"annotationFilter,omitempty"`

	// Filter sources managed by external-dns via annotation using label selector semantics
	// +optional
	LabelFilter *string `json:"labelFilter,omitempty"`
}

type SourceConfig struct {
	// TypeInfo contains the source type of the external dns
	// example:
	// type:
	//	 group:
	//	 version:
	// 	 kind:
	Type TypeInfo `json:"type"`

	// one of the below field is mandatory, according to the kind given in type info

	// For source type Node
	// +optional
	Node *NodeConfig `json:"node,omitempty"`

	// For source type Service
	// +optional
	Service *ServiceConfig `json:"service,omitempty"`

	// For source type Ingress
	// +optional
	Ingress *IngressConfig `json:"ingress,omitempty"`
}

// GenericSecretReference contains the information of the provider secret. Name is for secret name and CredentialKey is for specifying the key of the secret.
// It is considered optional where workload identity or IRSA (IAM Role for Service Account) is used, otherwise it is mandatory
type GenericSecretReference struct {
	// Name of the provider secret
	Name string `json:"name"`
	// credential key of the provider secret
	CredentialKey string `json:"credentialKey"`
}

// CloudflareSecretReference contains the name of the provider secret. The secret information may differ with respect to provider.
// It is considered optional where workload identity or IRSA (IAM Role for Service Account) is used, otherwise it is mandatory
type CloudflareSecretReference struct {
	// Name is the name of the secret that contains the provider credentials
	Name string `json:"name"`

	// first API token will be used, if it is not present then
	// API KEY and API Email will be used

	// +optional
	APITokenKey string `json:"apiTokenKey,omitempty"`

	// +optional
	APIKey string `json:"apiKey,omitempty"`

	// +optional
	APIEmailKey string `json:"apiEmailKey,omitempty"`
}

// ExternalDNSSpec defines the desired state of ExternalDNS
type ExternalDNSSpec struct {
	// Request timeout when calling Kubernetes API. 0s means no timeout
	// +optional
	RequestTimeout *time.Duration `json:"requestTimeout,omitempty"`

	// RELATED TO PROCESSING SOURCE
	// The resource types that are queried for endpoints; List of source. ex: source, ingress, node etc.
	// source:
	//    group: ""
	//    version: v1
	//    kind: Service
	Source SourceConfig `json:"source"`

	// If source is openshift router then you can pass the ingress controller name. Based on this name the
	// external dns will select the respective router from the route status and map that routeCanonicalHostname
	// to the route host while creating a CNAME record.
	// +optional
	OCRouterName *string `json:"ocRouterName,omitempty"`

	// Limit Gateways of route endpoints to a specific namespace
	// +optional
	GatewayNamespace *string `json:"gatewayNamespace,omitempty"`

	// Filter Gateways of Route endpoints via label selector
	// +optional
	GatewayLabelFilter *string `json:"gatewayLabelFilter,omitempty"`

	// The server to connect for connector source, valid only when using connector source
	// +optional
	ConnectorSourceServer *string `json:"connectorSourceServer,omitempty"`

	// Comma separated list of record types to manage (default: A, CNAME; supported: A,CNAME,NS)
	// +optional
	ManageDNSRecordTypes []string `json:"manageDNSRecordTypes,omitempty"`

	// Set globally a list of default IP address that will apply as a target instead of source addresses.
	// +optional
	DefaultTargets []string `json:"defaultTargets,omitempty"`

	//

	// RELATED TO PROVIDERS
	// The DNS provider where the DNS records will be created. (AWS, Cloudflare)
	Provider Provider `json:"provider"`

	// Limit possible target zones by a domain suffix
	// +optional
	DomainFilter []string `json:"domainFilter,omitempty"`

	// Exclude subdomains
	// +optional
	ExcludeDomains []string `json:"excludeDomains,omitempty"`

	// Filter target zones by hosted zone id
	// +optional
	ZoneIDFilter []string `json:"zoneIDFilter,omitempty"`

	// AWS provider information
	// +optional
	AWS *AWSProvider `json:"aws,omitempty"`

	// Cloudflare provider information
	// +optional
	Cloudflare *CloudflareProvider `json:"cloudflare,omitempty"`

	// Azure provider infomation
	// +optional
	Azure *AzureProvider `json:"azure,omitempty"`

	// Google provider
	// +optional
	Google *GoogleProvider `json:"google,omitempty"`

	//
	//POLICY INFORMATION
	//
	// Modify how DNS records are synchronized between sources and providers (default: sync, options: sync, upsert-only, create-only)
	// +optional
	Policy *Policy `json:"policy,omitempty"`

	//
	// REGISTRY information
	//
	// The registry implementation to use to keep track of DNS record ownership (default: txt, options: txt, noop, aws-sd)
	// +optional
	Registry *string `json:"registry,omitempty"`

	// When using the TXT registry, a name that identifies this instance of ExternalDNS (default: default)
	// +optional
	TXTOwnerID *string `json:"txtOwnerID,omitempty"`

	// When using the TXT registry, a custom string that's prefixed to each ownership DNS record (optional). Could
	// contain record type template like '%{record_type}-prefix-'. Mutual exclusive with txt-suffix!
	// +optional
	TXTPrefix *string `json:"txtPrefix,omitempty"`

	// When using the TXT registry, a custom string that's suffixed to the host portion of each ownership DNS
	// record. Could contain record type template like '-%{record_type}-suffix'. Mutual exclusive with txt-prefix!
	// +optional
	TXTSuffix *string `json:"txtSuffix,omitempty"`

	// When using the TXT registry, a custom string that's used instead of an asterisk for TXT records corresponding
	// to wildcard DNS records
	// +optional
	TXTWildcardReplacement *string `json:"txtWildcardReplacement,omitempty"`
}

// DNSRecord hold the DNS name and target address, if there are multiple target address then the addresses are joint by separator ';' between them (ex: 1:2:3:4;6:7:8:9)
type DNSRecord struct {
	// target is the list of target address
	// +optional
	Target string `json:"target,omitempty"`

	// dns name is the domain name for this record
	// +optional
	Name string `json:"name,omitempty"`
}

// ExternalDNSStatus defines the observed state of ExternalDNS
type ExternalDNSStatus struct {
	// Phase indicates the current state of the controller (ex: Failed,InProgress,Current)
	// +optional
	Phase ExternalDNSPhase `json:"phase,omitempty"`

	// ObservedGeneration indicates the latest generation that successfully reconciled
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions describe the current condition of the CRD
	// +optional
	Conditions []kmapi.Condition `json:"conditions,omitempty"`

	// DNSRecord is the list of records that this external dns operator registered
	// +optional
	DNSRecords []DNSRecord `json:"dnsRecords,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ExternalDNS is the Schema for the externaldns API
type ExternalDNS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalDNSSpec   `json:"spec,omitempty"`
	Status ExternalDNSStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:object:root=true

// ExternalDNSList contains a list of ExternalDNS
type ExternalDNSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalDNS `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalDNS{}, &ExternalDNSList{})
}
