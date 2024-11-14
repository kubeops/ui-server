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

// kubebuilder:validation:Enum:=sync;upsert-only;create-only
type Policy string

const (
	// Policy
	PolicySync       Policy = "sync"
	PolicyUpsertOnly Policy = "upsert-only"
	PolicyCreateOnly Policy = "create-only"
)

func (p Policy) String() string {
	return string(p)
}

type ExternalDNSPhase string

const (
	// ExternalDNSPhase
	ExternalDNSPhaseCurrent    ExternalDNSPhase = "Current"
	ExternalDNSPhaseFailed     ExternalDNSPhase = "Failed"
	ExternalDNSPhaseInProgress ExternalDNSPhase = "InProgress"
)

// kubebuilder:validation:Enum:=aws;cloudflare;azure;google
type Provider string

const (
	// Provider
	ProviderAWS        Provider = "aws"
	ProviderCloudflare Provider = "cloudflare"
	ProviderAzure      Provider = "azure"
	ProviderGoogle     Provider = "google"
)

func (p Provider) String() string {
	return string(p)
}

const (
	// ConditionType
	CreateAndRegisterWatcher = "CreateAndRegisterWatcher"
	GetProviderSecret        = "GetProviderSecret"
	CreateAndApplyPlan       = "CreateAndApplyPlan"
)
