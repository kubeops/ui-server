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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceKindCostReport = "CostReport"
	ResourceCostReport     = "costreport"
	ResourceCostReports    = "costreports"
)

// +genclient
// +genclient:nonNamespaced
// +genclient:onlyVerbs=create
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CostReport struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	Request *CostReportRequest `json:"request,omitempty"`
	// +optional
	Response *apiextensionsv1.JSON `json:"response,omitempty"`
}

type AccumulateOption string

const (
	AccumulateOptionNone    AccumulateOption = ""
	AccumulateOptionAll     AccumulateOption = "all"
	AccumulateOptionHour    AccumulateOption = "hour"
	AccumulateOptionDay     AccumulateOption = "day"
	AccumulateOptionWeek    AccumulateOption = "week"
	AccumulateOptionMonth   AccumulateOption = "month"
	AccumulateOptionQuarter AccumulateOption = "quarter"
)

type CostReportRequest struct {
	Window                                string           `json:"window,omitempty" schema:"window,omitempty"`
	Resolution                            string           `json:"resolution,omitempty" schema:"resolution,omitempty"`
	Step                                  string           `json:"step,omitempty" schema:"step,omitempty"`
	Aggregate                             []string         `json:"aggregate,omitempty" schema:"-"`
	AggregateList                         string           `json:"-" schema:"aggregate,omitempty"`
	IncludeIdle                           bool             `json:"includeIdle,omitempty" schema:"includeIdle,omitempty"`
	Accumulate                            bool             `json:"accumulate,omitempty" schema:"accumulate,omitempty"`
	AccumulateBy                          AccumulateOption `json:"accumulateBy,omitempty" schema:"accumulateBy,omitempty"`
	IdleByNode                            bool             `json:"idleByNode,omitempty" schema:"idleByNode,omitempty"`
	IncludeProportionalAssetResourceCosts bool             `json:"includeProportionalAssetResourceCosts,omitempty" schema:"includeProportionalAssetResourceCosts,omitempty"`
	IncludeAggregatedMetadata             bool             `json:"includeAggregatedMetadata,omitempty" schema:"includeAggregatedMetadata,omitempty"`
}
