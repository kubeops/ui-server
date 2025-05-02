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

import gwv1 "sigs.k8s.io/gateway-api/apis/v1"

type ServiceRef struct {
	// Name is the name of the referent.
	Name gwv1.ObjectName `json:"name"`

	// Namespace is the namespace of the referenced object. When unspecified, the local
	// namespace is inferred.
	//
	// Note that when a namespace different than the local namespace is specified,
	// a ReferenceGrant object is required in the referent namespace to allow that
	// namespace's owner to accept the reference. See the ReferenceGrant
	// documentation for details.
	//
	// Support: Core
	//
	// +optional
	Namespace *gwv1.Namespace `json:"namespace,omitempty"`
}
type RouteFilter struct {
}

type TelemetryRefence struct {
	Reference string `json:"ref,omitempty"`
}

type TelemetryConfig struct {
	Name         string       `json:"name,omitempty"`
	SamplingRate int          `json:"samplingRate,omitempty"`
	Provider     OtelProvider `json:"provider,omitempty"`
}

type OtelProvider struct {
	Host string `json:"host,omitempty"`
	Port int32  `json:"port,omitempty"`
}
