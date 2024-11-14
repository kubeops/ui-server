/*
Copyright 2023.

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
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=gateway-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PostgresRoute provides a way to route TCP requests. When combined with a Gateway
// listener, it can be used to forward connections on the port specified by the
// listener to a set of backends specified by the PostgresRoute.
type PostgresRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of PostgresRoute.
	Spec PostgresRouteSpec `json:"spec"`

	// Status defines the current state of PostgresRoute.
	Status PostgresRouteStatus `json:"status,omitempty"`
}

// PostgresRouteSpec defines the desired state of PostgresRoute
type PostgresRouteSpec struct {
	gwv1.CommonRouteSpec `json:",inline"`

	// Hostnames defines a set of SNI names that should match against the
	// SNI attribute of TLS ClientHello message in TLS handshake. This matches
	// the RFC 1123 definition of a hostname with 2 notable exceptions:
	//
	// 1. IPs are not allowed in SNI names per RFC 6066.
	// 2. A hostname may be prefixed with a wildcard label (`*.`). The wildcard
	//    label must appear by itself as the first label.
	//
	// If a hostname is specified by both the Listener and KafkaRoute, there
	// must be at least one intersecting hostname for the KafkaRoute to be
	// attached to the Listener. For example:
	//
	// * A Listener with `test.example.com` as the hostname matches KafkaRoutes
	//   that have either not specified any hostnames, or have specified at
	//   least one of `test.example.com` or `*.example.com`.
	// * A Listener with `*.example.com` as the hostname matches KafkaRoutes
	//   that have either not specified any hostnames or have specified at least
	//   one hostname that matches the Listener hostname. For example,
	//   `test.example.com` and `*.example.com` would both match. On the other
	//   hand, `example.com` and `test.example.net` would not match.
	//
	// If both the Listener and KafkaRoute have specified hostnames, any
	// KafkaRoute hostnames that do not match the Listener hostname MUST be
	// ignored. For example, if a Listener specified `*.example.com`, and the
	// KafkaRoute specified `test.example.com` and `test.example.net`,
	// `test.example.net` must not be considered for a match.
	//
	// If both the Listener and KafkaRoute have specified hostnames, and none
	// match with the criteria above, then the KafkaRoute is not accepted. The
	// implementation must raise an 'Accepted' Condition with a status of
	// `False` in the corresponding RouteParentStatus.
	//
	// Support: Core
	//
	// +optional
	// +kubebuilder:validation:MaxItems=16
	Hostnames []gwv1.Hostname `json:"hostnames,omitempty"`

	// Rules are a list of TCP matchers and actions.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Rules []PostgresRouteRule `json:"rules"`

	// +optional
	Telemetry TelemetryRefence `json:"telemetry"`
}

// PostgresRouteStatus defines the observed state of PostgresRoute
type PostgresRouteStatus struct {
	gwv1.RouteStatus `json:",inline"`
}

// PostgresRouteRule is the configuration for a given rule.
type PostgresRouteRule struct {
	// Filters define the filters that are applied to requests that match
	// this rule.
	//
	// The effects of ordering of multiple behaviors are currently unspecified.
	// This can change in the future based on feedback during the alpha stage.
	//
	// Conformance-levels at this level are defined based on the type of filter:
	//
	// - ALL core filters MUST be supported by all implementations.
	// - Implementers are encouraged to support extended filters.
	// - Implementation-specific custom filters have no API guarantees across
	//   implementations.
	//
	// Specifying a core filter multiple times has unspecified or
	// implementation-specific conformance.
	//
	// All filters are expected to be compatible with each other except for the
	// URLRewrite and RequestRedirect filters, which may not be combined. If an
	// implementation can not support other combinations of filters, they must clearly
	// document that limitation. In all cases where incompatible or unsupported
	// filters are specified, implementations MUST add a warning condition to status.
	//
	// Support: Core
	//
	// +optional
	// +kubebuilder:validation:MaxItems=16
	Filters []RouteFilter `json:"filters,omitempty"`

	// BackendRefs defines the backend(s) where matching requests should be
	// sent. If unspecified or invalid (refers to a non-existent resource or a
	// Service with no endpoints), the underlying implementation MUST actively
	// reject connection attempts to this backend. Connection rejections must
	// respect weight; if an invalid backend is requested to have 80% of
	// connections, then 80% of connections must be rejected instead.
	//
	// Support: Core for Kubernetes Service
	//
	// Support: Extended for Kubernetes ServiceImport
	//
	// Support: Implementation-specific for any other resource
	//
	// Support for weight: Extended
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	BackendRefs []gwv1.BackendRef `json:"backendRefs,omitempty"`
}

// +kubebuilder:object:root=true

// PostgresRouteList contains a list of PostgresRoute
type PostgresRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresRoute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresRoute{}, &PostgresRouteList{})
}
