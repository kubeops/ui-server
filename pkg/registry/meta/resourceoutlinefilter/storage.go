/*
Copyright AppsCode Inc. and Contributors

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

package resourceoutlinefilter

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	kerr "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	"kmodules.xyz/resource-metadata/apis/meta"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/resourceoutlines"
	"kmodules.xyz/resource-metadata/pkg/layouts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Client
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

func NewStorage(kc client.Client) *Storage {
	return &Storage{
		kc: kc,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    rsapi.SchemeGroupVersion.Group,
			Resource: rsapi.ResourceResourceOutlineFilters,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindResourceOutlineFilter)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rsapi.ResourceKindResourceOutlineFilter)
}

// Getter
func (r *Storage) New() runtime.Object {
	return &rsapi.ResourceOutlineFilter{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	obj, err := resourceoutlines.LoadByName(name)
	if err != nil {
		return nil, kerr.NewNotFound(schema.GroupResource{Group: meta.GroupName, Resource: rsapi.ResourceKindResourceOutline}, name)
	}
	filter, err := layouts.GetResourceOutlineFilter(r.kc, obj)
	if err != nil {
		return nil, kerr.NewInternalError(err)
	}
	return &rsapi.ResourceOutlineFilter{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: filter.Name,
		},
		Spec: filter.Spec,
	}, nil
}

// Lister
func (r *Storage) NewList() runtime.Object {
	return &rsapi.ResourceOutlineFilterList{}
}

func (r *Storage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	if options.FieldSelector != nil && !options.FieldSelector.Empty() {
		return nil, kerr.NewBadRequest("fieldSelector is not a supported")
	}

	objs := resourceoutlines.List()

	if options.Continue != "" {
		start, err := strconv.Atoi(options.Continue)
		if err != nil {
			return nil, kerr.NewBadRequest(fmt.Sprintf("invalid continue option, err:%v", err))
		}
		if start > len(objs) {
			return r.NewList(), nil
		}
		objs = objs[start:]
	}
	if options.Limit > 0 && int64(len(objs)) > options.Limit {
		objs = objs[:options.Limit]
	}

	items := make([]rsapi.ResourceOutlineFilter, 0, len(objs))
	for _, obj := range objs {
		if options.LabelSelector != nil && !options.LabelSelector.Matches(labels.Set(obj.GetLabels())) {
			continue
		}
		filter, err := layouts.GetResourceOutlineFilter(r.kc, &obj)
		if err != nil {
			return nil, kerr.NewInternalError(err)
		}
		items = append(items, rsapi.ResourceOutlineFilter{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: filter.Name,
			},
			Spec: filter.Spec,
		})
	}

	return &rsapi.ResourceOutlineFilterList{Items: items}, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}
