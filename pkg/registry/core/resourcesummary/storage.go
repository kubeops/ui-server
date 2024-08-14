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
	"sort"
	"strings"
	"time"

	"kubeops.dev/ui-server/pkg/shared"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"kmodules.xyz/apiversion"
	kmapi "kmodules.xyz/client-go/api/v1"
	clustermeta "kmodules.xyz/client-go/cluster"
	rscoreapi "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
	resourcemetrics "kmodules.xyz/resource-metrics"
	"kmodules.xyz/resource-metrics/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Client
	dc        discovery.DiscoveryInterface
	clusterID string
	a         authorizer.Authorizer
	convertor rest.TableConvertor
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Lister                   = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client, dc discovery.DiscoveryInterface, clusterID string, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc:        kc,
		dc:        dc,
		clusterID: clusterID,
		a:         a,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    rscoreapi.GroupName,
			Resource: rscoreapi.ResourceResourceSummaries,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rscoreapi.SchemeGroupVersion.WithKind(rscoreapi.ResourceKindResourceSummary)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rscoreapi.ResourceKindResourceSummary)
}

func (r *Storage) New() runtime.Object {
	return &rscoreapi.ResourceSummary{}
}

func (r *Storage) Destroy() {}

func (r *Storage) NewList() runtime.Object {
	return &rscoreapi.ResourceSummaryList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	cmeta, err := clustermeta.ClusterMetadata(r.kc)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(r.dc))

	selector := shared.NewGroupKindSelector(options.LabelSelector)
	now := time.Now()

	gvks := make(map[schema.GroupKind]string)
	for _, gvk := range api.RegisteredTypes() {
		if !selector.Matches(gvk.GroupKind()) {
			continue
		}
		gk := gvk.GroupKind()
		if v, exists := gvks[gk]; exists {
			if apiversion.MustCompare(v, gvk.Version) < 0 {
				gvks[gk] = gvk.Version
			}
		} else {
			gvks[gk] = gvk.Version
		}
	}

	items := make([]rscoreapi.ResourceSummary, 0)
	for gk, v := range gvks {
		if !selector.Matches(gk) {
			continue
		}

		mapping, err := mapper.RESTMapping(gk, v)
		if meta.IsNoMatchError(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		apiType := kmapi.NewResourceID(mapping)

		attrs := authorizer.AttributesRecord{
			User:            user,
			Verb:            "get",
			Namespace:       ns,
			APIGroup:        mapping.Resource.Group,
			Resource:        mapping.Resource.Resource,
			Name:            "",
			ResourceRequest: true,
		}

		summary := rscoreapi.ResourceSummary{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:              gk.String(),
				Namespace:         ns,
				CreationTimestamp: metav1.NewTime(now),
				UID:               types.UID(uuid.Must(uuid.NewUUID()).String()),
			},
			Spec: rscoreapi.ResourceSummarySpec{
				Cluster: *cmeta,
				APIType: *apiType,
				// TotalResource: core.ResourceRequirements{},
				// AppResource:   core.ResourceRequirements{},
				Count: 0,
			},
		}

		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gk.Group,
			Version: v,
			Kind:    gk.Kind,
		})
		if err := r.kc.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, err
		}

		// hasPermission to check if the user has permission to list the resources
		hasPermission := false
		for _, item := range list.Items {
			attrs.Name = item.GetName()
			attrs.Namespace = item.GetNamespace()
			decision, _, err := r.a.Authorize(ctx, attrs)
			if err != nil {
				return nil, apierrors.NewInternalError(err)
			}
			if decision != authorizer.DecisionAllow {
				continue
			}

			hasPermission = true
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

		if hasPermission {
			summary.Spec.Count = len(list.Items)
		}
		items = append(items, summary)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Spec.APIType.Group != items[j].Spec.APIType.Group {
			return items[i].Spec.APIType.Group < items[j].Spec.APIType.Group
		}
		return items[i].Spec.APIType.Kind < items[j].Spec.APIType.Kind
	})

	result := rscoreapi.ResourceSummaryList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    items,
	}

	return &result, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}
