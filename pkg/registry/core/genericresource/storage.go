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

package genericresource

import (
	"context"
	"sort"
	"strings"

	"kubeops.dev/ui-server/pkg/shared"

	"github.com/pkg/errors"
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
	"kmodules.xyz/apiversion"
	kmapi "kmodules.xyz/client-go/api/v1"
	clustermeta "kmodules.xyz/client-go/cluster"
	rscoreapi "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
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
	_ rest.Getter                   = &Storage{}
	_ rest.Lister                   = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client, clusterID string, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc:        kc,
		clusterID: clusterID,
		a:         a,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    rscoreapi.GroupName,
			Resource: rscoreapi.ResourceGenericResources,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rscoreapi.SchemeGroupVersion.WithKind(rscoreapi.ResourceKindGenericResource)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rscoreapi.ResourceKindGenericResource)
}

func (r *Storage) New() runtime.Object {
	return &rscoreapi.GenericResource{}
}

func (r *Storage) Destroy() {}

func (r *Storage) NewList() runtime.Object {
	return &rscoreapi.GenericResourceList{}
}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
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

	objName, gk, err := rscoreapi.ParseGenericResourceName(name)
	if err != nil {
		return nil, apierrors.NewBadRequest(err.Error())
	}
	mapping, err := r.kc.RESTMapper().RESTMapping(gk)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	rid := kmapi.NewResourceID(mapping)

	attrs := authorizer.AttributesRecord{
		User:            user,
		Verb:            "get",
		Namespace:       ns,
		APIGroup:        mapping.Resource.Group,
		Resource:        mapping.Resource.Resource,
		Name:            objName,
		ResourceRequest: true,
	}
	decision, why, err := r.a.Authorize(ctx, attrs)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	if decision != authorizer.DecisionAllow {
		return nil, apierrors.NewForbidden(mapping.Resource.GroupResource(), objName, errors.New(why))
	}

	var obj unstructured.Unstructured
	obj.SetGroupVersionKind(mapping.GroupVersionKind)
	err = r.kc.Get(ctx, client.ObjectKey{Namespace: ns, Name: objName}, &obj)
	if err != nil {
		return nil, err
	}

	return rscoreapi.ToGenericResource(&obj, rid, cmeta)
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}
	selector := shared.NewGroupKindSelector(options.LabelSelector)

	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	cmeta, err := clustermeta.ClusterMetadata(r.kc)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	items := make([]rscoreapi.GenericResource, 0)
	for _, gvk := range api.RegisteredTypes() {
		if !selector.Matches(gvk.GroupKind()) {
			continue
		}

		mapping, err := r.kc.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
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

		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(gvk)
		if err := r.kc.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, err
		}
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

			genres, err := rscoreapi.ToGenericResource(&item, apiType, cmeta)
			if err != nil {
				return nil, err
			}
			items = append(items, *genres)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		gvk_i := items[i].GetObjectKind().GroupVersionKind()
		gvk_j := items[j].GetObjectKind().GroupVersionKind()
		if gvk_i.Group != gvk_j.Group {
			return gvk_i.Group < gvk_j.Group
		}
		if gvk_i.Version != gvk_j.Version {
			diff, _ := apiversion.Compare(gvk_i.Version, gvk_j.Version)
			return diff < 0
		}
		if gvk_i.Kind != gvk_j.Kind {
			return gvk_i.Kind < gvk_j.Kind
		}
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
	})

	result := rscoreapi.GenericResourceList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    items,
	}

	return &result, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}
