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

package ResourceSummary

import (
	"context"
	"time"

	uiv1alpha1 "kubeops.dev/ui-server/apis/ui/v1alpha1"
	"kubeops.dev/ui-server/pkg/shared"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"kmodules.xyz/client-go/tools/clusterid"
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

var _ rest.GroupVersionKindProvider = &Storage{}
var _ rest.Scoper = &Storage{}
var _ rest.Lister = &Storage{}

func NewStorage(kc client.Client, clusterID string, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc:        kc,
		clusterID: clusterID,
		a:         a,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    uiv1alpha1.GroupName,
			Resource: uiv1alpha1.ResourceResourceSummaries,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return uiv1alpha1.GroupVersion.WithKind(uiv1alpha1.ResourceKindResourceSummary)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) New() runtime.Object {
	return &uiv1alpha1.ResourceSummary{}
}

func (r *Storage) NewList() runtime.Object {
	return &uiv1alpha1.ResourceSummaryList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}

	apiGroups := shared.GetAPIGroups(options.LabelSelector)
	if apiGroups.Len() == 0 {
		return &uiv1alpha1.ResourceSummaryList{}, nil
	}

	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	now := time.Now()

	items := make([]uiv1alpha1.ResourceSummary, 0)
	for _, gvk := range api.RegisteredTypes() {
		if apiGroups.Len() > 0 && !apiGroups.Has(gvk.Group) {
			continue
		}

		mapping, err := r.kc.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
		if meta.IsNoMatchError(err) {
			continue
		} else if err != nil {
			return nil, err
		}

		attrs := authorizer.AttributesRecord{
			User:      user,
			Verb:      "get",
			Namespace: ns,
			APIGroup:  mapping.Resource.Group,
			Resource:  mapping.Resource.Resource,
			Name:      "",
		}

		summary := uiv1alpha1.ResourceSummary{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:              gvk.GroupKind().String(),
				Namespace:         ns,
				CreationTimestamp: metav1.NewTime(now),
			},
			Spec: uiv1alpha1.ResourceSummarySpec{
				ClusterName: clusterid.ClusterName(),
				ClusterID:   r.clusterID,
				APIGroup:    gvk.Group,
				Kind:        gvk.Kind,
				// TotalResource: core.ResourceRequirements{},
				// AppResource:   core.ResourceRequirements{},
				Count: 0,
			},
		}

		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(gvk)
		if err := r.kc.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			attrs.Name = item.GetName()
			decision, _, err := r.a.Authorize(ctx, attrs)
			if err != nil {
				return nil, apierrors.NewInternalError(err)
			}
			if decision != authorizer.DecisionAllow {
				continue
			}

			content := item.UnstructuredContent()
			{
				rv, err := resourcemetrics.TotalResourceRequests(content)
				if err != nil {
					return nil, err
				}
				summary.Spec.TotalResource.Requests = api.AddResourceList(summary.Spec.TotalResource.Requests, rv)
			}
			{
				rv, err := resourcemetrics.TotalResourceLimits(content)
				if err != nil {
					return nil, err
				}
				summary.Spec.TotalResource.Limits = api.AddResourceList(summary.Spec.TotalResource.Limits, rv)
			}
			{
				rv, err := resourcemetrics.AppResourceRequests(content)
				if err != nil {
					return nil, err
				}
				summary.Spec.AppResource.Requests = api.AddResourceList(summary.Spec.AppResource.Requests, rv)
			}
			{
				rv, err := resourcemetrics.AppResourceLimits(content)
				if err != nil {
					return nil, err
				}
				summary.Spec.AppResource.Limits = api.AddResourceList(summary.Spec.AppResource.Limits, rv)
			}
		}

		summary.Spec.Count = len(list.Items)
		items = append(items, summary)
	}

	result := uiv1alpha1.ResourceSummaryList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    items,
	}
	result.ListMeta.SelfLink = ""

	return &result, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}
