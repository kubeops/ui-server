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
	"strings"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clustermeta "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/resource-metadata/apis/management/v1alpha1"
	"kmodules.xyz/resource-metrics/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func deductOldDbObjectResourceUsageFromProjectQuota(kc client.Client, u unstructured.Unstructured, pq *v1alpha1.ProjectQuota) error {
	oldDbObj := &unstructured.Unstructured{}
	oldDbObj.SetGroupVersionKind(u.GroupVersionKind())

	err := kc.Get(context.TODO(), types.NamespacedName{Name: u.GetName(), Namespace: u.GetNamespace()}, oldDbObj)
	if err != nil {
		return err
	}

	if err := deductDbObjResourceUsageFromProjectQuota(oldDbObj.UnstructuredContent(), pq); err != nil {
		return err
	}

	return nil
}

func deductDbObjResourceUsageFromProjectQuota(dbObj map[string]any, pq *v1alpha1.ProjectQuota) error {
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

func getApplicableQuotas(kc client.Client, ns string) ([]*v1alpha1.ProjectQuota, error) {
	var names []string
	if clustermeta.IsRancherManaged(kc.RESTMapper()) {
		projectId, _, err := clustermeta.GetProjectId(kc, ns)
		if err != nil {
			return nil, err
		}
		names = append(names, projectId)
	}
	names = append(names, ns)

	var result []*v1alpha1.ProjectQuota
	for _, name := range names {
		var pj v1alpha1.ProjectQuota
		err := kc.Get(context.TODO(), client.ObjectKey{Name: name}, &pj)
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		} else if err == nil {
			result = append(result, &pj)
		}
	}
	return result, nil
}

func getGVK(obj map[string]any) schema.GroupVersionKind {
	var unObj unstructured.Unstructured
	unObj.SetUnstructuredContent(obj)

	return unObj.GroupVersionKind()
}
