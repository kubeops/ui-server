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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	corev1alpha1 "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
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
			Group:    corev1alpha1.GroupName,
			Resource: corev1alpha1.ResourceResourceCalculators,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return corev1alpha1.SchemeGroupVersion.WithKind(corev1alpha1.ResourceKindResourceCalculator)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) New() runtime.Object {
	return &corev1alpha1.ResourceCalculator{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*corev1alpha1.ResourceCalculator)
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

	resp, err := ToGenericResource(&u, rid)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	in.Response = resp
	return in, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func ToGenericResource(item *unstructured.Unstructured, apiType *kmapi.ResourceID) (*corev1alpha1.ResourceCalculatorResponse, error) {
	content := item.UnstructuredContent()

	var genres corev1alpha1.ResourceCalculatorResponse
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
	}
	return &genres, nil
}
