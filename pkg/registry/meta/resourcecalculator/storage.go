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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metadata/apis/management/v1alpha1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	resourcemetrics "kmodules.xyz/resource-metrics"
	"kmodules.xyz/resource-metrics/api"
	opsv1alpha1 "kmodules.xyz/resource-metrics/ops.kubedb.com/v1alpha1"
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
	err := json.Unmarshal(in.Request.Resource.Raw, &u)
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
	// Wrap referenced db resource with the OpsRequest object
	if rid.Group == "ops.kubedb.com" {
		if err = wrapReferencedDBResourceWithOpsReqObject(r.kc, &u); err != nil {
			return nil, err
		}
	} else if in.Request.Edit {
		if err := deductOldDbObjectResourceUsageFromProjectQuota(r.kc, u, pq); err != nil {
			return nil, err
		}
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
			rv, err := quota(content, pq)
			if err != nil {
				return nil, err
			}
			genres.Quota = *rv
		}
	}
	return &genres, nil
}

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
		if err := deductDbObjResourceUsageFromProjectQuota(dbObj, pq); err != nil {
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
