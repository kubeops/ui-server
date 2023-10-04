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
	"strings"

	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	clustermeta "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/resource-metadata/apis/management/v1alpha1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	resourcemetrics "kmodules.xyz/resource-metrics"
	"kmodules.xyz/resource-metrics/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Client
	clusterID string
	a         authorizer.Authorizer
	convertor rest.TableConvertor
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
)

func NewStorage(kc client.Client, clusterID string, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc:        kc,
		clusterID: clusterID,
		a:         a,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    rsapi.SchemeGroupVersion.Group,
			Resource: rsapi.ResourceResourceCalculators,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindResourceCalculator)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &rsapi.ResourceCalculator{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*rsapi.ResourceCalculator)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}

	var u unstructured.Unstructured
	err := json.Unmarshal(in.Request.Raw, &u)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	gvk := u.GetObjectKind().GroupVersionKind()
	var versions []string
	if gvk.Version != "" {
		versions = append(versions, gvk.Version)
	}
	mapping, err := r.kc.RESTMapper().RESTMapping(gvk.GroupKind(), versions...)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	rid := kmapi.NewResourceID(mapping)
	pq, err := getProjectQuota(r.kc, u.GetNamespace())
	if err != nil {
		return nil, err
	}

	resp, err := ToGenericResource(&u, rid, pq)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	in.Response = resp
	return in, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func ToGenericResource(item *unstructured.Unstructured, apiType *kmapi.ResourceID, pq *v1alpha1.ProjectQuota) (*rsapi.ResourceCalculatorResponse, error) {
	content := item.UnstructuredContent()

	var genres rsapi.ResourceCalculatorResponse
	genres.APIType = *apiType
	{
		if v, ok, _ := unstructured.NestedString(content, "spec", "version"); ok {
			genres.Version = v
		}
	}
	if api.IsRegistered(apiType.GroupVersionKind()) {
		{
			rv, err := resourcemetrics.Replicas(content)
			if err != nil {
				return nil, err
			}
			genres.Replicas = rv
		}
		{
			rv, err := resourcemetrics.RoleReplicas(content)
			if err != nil {
				return nil, err
			}
			genres.RoleReplicas = rv
		}
		{
			rv, err := resourcemetrics.Mode(content)
			if err != nil {
				return nil, err
			}
			genres.Mode = rv
		}
		{
			rv, err := resourcemetrics.TotalResourceRequests(content)
			if err != nil {
				return nil, err
			}
			genres.TotalResource.Requests = rv
		}
		{
			rv, err := resourcemetrics.TotalResourceLimits(content)
			if err != nil {
				return nil, err
			}
			genres.TotalResource.Limits = rv
		}
		{
			rv, err := resourcemetrics.AppResourceRequests(content)
			if err != nil {
				return nil, err
			}
			genres.AppResource.Requests = rv
		}
		{
			rv, err := resourcemetrics.AppResourceLimits(content)
			if err != nil {
				return nil, err
			}
			genres.AppResource.Limits = rv
		}
		{
			rv, err := resourcemetrics.RoleResourceRequests(content)
			if err != nil {
				return nil, err
			}
			genres.RoleResourceRequests = rv
		}
		{
			rv, err := resourcemetrics.RoleResourceLimits(content)
			if err != nil {
				return nil, err
			}
			genres.RoleResourceLimits = rv
		}
		{
			rv, err := quota(content, pq, apiType)
			if err != nil {
				return nil, err
			}
			genres.Quota = *rv
		}
	}
	return &genres, nil
}

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
		if quota.QuotaStatus.Result != v1alpha1.ResultSuccess {
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
