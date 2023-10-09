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

package resourceCalculator

import (
	"context"
	"fmt"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kmapi "kmodules.xyz/client-go/api/v1"
	clustermeta "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/resource-metadata/apis/management/v1alpha1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metrics/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func quota(obj map[string]interface{}, pq *v1alpha1.ProjectQuota, apiType *kmapi.ResourceID) (*rsapi.QuotaDecision, error) {
	qd := &rsapi.QuotaDecision{
		Decision:   rsapi.DecisionAllow,
		Violations: make([]string, 0),
	}
	if pq == nil {
		qd.Decision = rsapi.DecisionNoOpinion
		return qd, nil
	}

	c, err := api.Load(obj)
	if err != nil {
		return nil, err
	}
	requests, err := c.AppResourceRequests(obj)
	if err != nil {
		return nil, err
	}
	limits, err := c.AppResourceLimits(obj)
	if err != nil {
		return nil, err
	}

	for _, quota := range pq.Status.Quotas {
		if quota.Result != v1alpha1.ResultSuccess {
			continue
		}
		if quota.Group == apiType.Group {
			if quota.Kind != "" && quota.Kind != apiType.Kind {
				continue
			}
			hardRequests, hardLimits := extractRequestsLimits(quota.Hard)
			usedRequests, usedLimits := extractRequestsLimits(quota.Used)

			totRequestsUsage := api.AddResourceList(requests, usedRequests)
			for rn, usageQuan := range totRequestsUsage {
				hr, found := hardRequests[rn]
				if !found {
					continue
				}
				if usageQuan.Cmp(hr) > 0 {
					r := requests[rn]
					u := usedRequests[rn]
					l := hardRequests[rn]

					qd.Decision = rsapi.DecisionDeny
					qd.Violations = append(qd.Violations,
						fmt.Sprintf("Project quota exceeded. Requested: requests.%s=%s, Used: requests.%s=%s, Limited: requests.%s=%s", rn, r.String(), rn, u.String(), rn, l.String()))
				}
			}

			totLimitsUsage := api.AddResourceList(limits, usedLimits)
			for rn, usageQuan := range totLimitsUsage {
				hl, found := hardLimits[rn]
				if !found {
					continue
				}
				if usageQuan.Cmp(hl) > 0 {
					r := limits[rn]
					u := usedLimits[rn]
					l := hardLimits[rn]

					qd.Decision = rsapi.DecisionDeny
					qd.Violations = append(qd.Violations,
						fmt.Sprintf("Project quota exceeded. Requested: limits.%s=%s, Used: limits.%s=%s, Limited: limits.%s=%s", rn, r.String(), rn, u.String(), rn, l.String()))
				}
			}
		}
	}

	return qd, nil
}

func extractRequestsLimits(res core.ResourceList) (core.ResourceList, core.ResourceList) {
	requests := core.ResourceList{}
	limits := core.ResourceList{}

	for fullName, quan := range res {
		identifier, name, found := strings.Cut(fullName.String(), ".")
		if !found {
			continue
		}

		if identifier == "requests" {
			requests[core.ResourceName(name)] = quan
		} else {
			limits[core.ResourceName(name)] = quan
		}
	}

	return requests, limits
}

func getProjectQuota(kc client.Client, ns string) (*v1alpha1.ProjectQuota, error) {
	projectId, _, err := clustermeta.GetProjectId(kc, ns)
	if err != nil {
		return nil, err
	}
	var pj v1alpha1.ProjectQuota
	err = kc.Get(context.TODO(), client.ObjectKey{Name: projectId}, &pj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &pj, nil
}
