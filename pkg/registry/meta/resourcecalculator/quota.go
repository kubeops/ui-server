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

package resourcecalculator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clustermeta "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/resource-metadata/apis/management/v1alpha1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metrics/api"
	opsv1alpha1 "kmodules.xyz/resource-metrics/ops.kubedb.com/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func quota(obj map[string]interface{}, pq *v1alpha1.ProjectQuota) (*rsapi.QuotaDecision, error) {
	qd := &rsapi.QuotaDecision{
		Decision:   rsapi.DecisionAllow,
		Violations: make([]string, 0),
	}
	if pq == nil {
		qd.Decision = rsapi.DecisionNoOpinion
		return qd, nil
	}

	gvk := getGVK(obj)
	if gvk.Group == "ops.kubedb.com" {
		opsPathMapper, err := opsv1alpha1.LoadOpsPathMapper(obj)
		if err != nil {
			return nil, err
		}
		dbObj, err := extractReferencedObject(obj, opsPathMapper.GetReferencedDbObjectPath()...)
		if err != nil {
			return nil, err
		}
		if err := deductRefDbObjResourceUsageFromProjectQuota(obj, pq); err != nil {
			return nil, err
		}
		gvk = getGVK(dbObj)
	}

	c, err := api.Load(obj)
	if err != nil {
		return nil, err
	}
	dbRequests, err := c.AppResourceRequests(obj)
	if err != nil {
		return nil, err
	}
	dbLimits, err := c.AppResourceLimits(obj)
	if err != nil {
		return nil, err
	}
	dbDemand := mergeRequestsLimits(dbRequests, dbLimits)

	for _, quota := range pq.Status.Quotas {
		if quota.Result != v1alpha1.ResultSuccess {
			continue
		}
		if quota.Group == gvk.Group {
			if quota.Kind != "" && quota.Kind != gvk.Kind {
				continue
			}
			newUsed := api.AddResourceList(quota.Used, dbDemand)
			for rk, newUsed := range newUsed {
				hard, found := quota.Hard[rk]
				if !found {
					continue
				}
				if newUsed.Cmp(hard) > 0 {
					dd := dbDemand[rk]
					du := quota.Used[rk]
					dh := quota.Hard[rk]

					qd.Decision = rsapi.DecisionDeny
					qd.Violations = append(qd.Violations,
						fmt.Sprintf("Project quota exceeded. Requested: %s=%s, Used: %s=%s, Limited: %s=%s", rk, dd.String(), rk, du.String(), rk, dh.String()))
				}
			}
		}
	}

	return qd, nil
}

func deductRefDbObjResourceUsageFromProjectQuota(dbObj map[string]interface{}, pq *v1alpha1.ProjectQuota) error {
	c, err := api.Load(dbObj)
	if err != nil {
		return err
	}
	dbRequests, err := c.AppResourceRequests(dbObj)
	if err != nil {
		return err
	}
	dbLimits, err := c.AppResourceLimits(dbObj)
	if err != nil {
		return err
	}
	dbDemand := mergeRequestsLimits(dbRequests, dbLimits)

	gvk := getGVK(dbObj)
	for i, quota := range pq.Status.Quotas {
		if quota.Result != v1alpha1.ResultSuccess {
			continue
		}
		if quota.Group == gvk.Group {
			if quota.Kind != "" && quota.Kind != gvk.Kind {
				continue
			}
			quota.Used = api.SubtractResourceList(quota.Used, dbDemand)
		}
		pq.Status.Quotas[i].Used = quota.Used
	}

	return nil
}

func mergeRequestsLimits(requests, limits core.ResourceList) core.ResourceList {
	rl := make(core.ResourceList)
	for k, r := range requests {
		_, _, found := strings.Cut(k.String(), ".")
		if !found {
			rl["requests."+k] = r
		} else {
			rl[k] = r
		}
	}
	for k, l := range limits {
		_, _, found := strings.Cut(k.String(), ".")
		if !found {
			rl["limits."+k] = l
		} else {
			rl[k] = l
		}
	}

	return rl
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

func getGVK(obj map[string]interface{}) schema.GroupVersionKind {
	var unObj unstructured.Unstructured
	unObj.SetUnstructuredContent(obj)

	return unObj.GroupVersionKind()
}

func extractReferencedObject(opsObj map[string]interface{}, refDbPath ...string) (map[string]interface{}, error) {
	if len(refDbPath) == 0 {
		refDbPath = []string{"spec", "databaseRef", "referencedDB"}
	}
	dbObj, found, _ := unstructured.NestedMap(opsObj, refDbPath...)
	if !found {
		return nil, errors.New("referenced db object not found")
	}

	return dbObj, nil
}
