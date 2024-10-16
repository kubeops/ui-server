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

package resourceoutline

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
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/clusterprofiles"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Reader
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

func NewStorage(kc client.Reader) *Storage {
	return &Storage{
		kc: kc,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    uiapi.SchemeGroupVersion.Group,
			Resource: uiapi.ResourceClusterProfiles,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return uiapi.SchemeGroupVersion.WithKind(uiapi.ResourceKindClusterProfile)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(uiapi.ResourceKindClusterProfile)
}

// Getter
func (r *Storage) New() runtime.Object {
	return &uiapi.ClusterProfile{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	obj, err := clusterprofiles.LoadByName(r.kc, name)
	if err != nil {
		return nil, kerr.NewNotFound(schema.GroupResource{Group: meta.GroupName, Resource: uiapi.ResourceKindClusterProfile}, name)
	}
	return obj, err
}

// Lister
func (r *Storage) NewList() runtime.Object {
	return &uiapi.ClusterProfileList{}
}

func (r *Storage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	if options.FieldSelector != nil && !options.FieldSelector.Empty() {
		return nil, kerr.NewBadRequest("fieldSelector is not a supported")
	}

	objs, err := clusterprofiles.List(r.kc)
	if err != nil {
		return nil, err
	}
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

	items := make([]uiapi.ClusterProfile, 0, len(objs))
	for _, obj := range objs {
		if options.LabelSelector != nil && !options.LabelSelector.Matches(labels.Set(obj.GetLabels())) {
			continue
		}
		items = append(items, obj)
	}

	return &uiapi.ClusterProfileList{Items: items}, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}
