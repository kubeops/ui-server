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

package podview

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	mgmtapi "kmodules.xyz/client-go/apis/management/v1alpha1"
	clustermeta "kmodules.xyz/client-go/cluster"
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
	_ rest.Lister                   = &Storage{}
	_ rest.Getter                   = &Storage{}
)

func NewStorage(kc client.Client) *Storage {
	s := &Storage{
		kc: kc,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    mgmtapi.GroupVersion.Group,
			Resource: mgmtapi.ResourceProjects,
		}),
	}
	return s
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return mgmtapi.GroupVersion.WithKind(mgmtapi.ResourceKindProject)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &mgmtapi.Project{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	if clustermeta.IsRancherManaged(r.kc.RESTMapper()) {
		return clustermeta.GetRancherProject(r.kc, name)
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{
		Group:    mgmtapi.GroupVersion.Group,
		Resource: mgmtapi.ResourceProjects,
	}, name)
}

func (r *Storage) NewList() runtime.Object {
	return &mgmtapi.ProjectList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	var projects []mgmtapi.Project
	var err error
	if clustermeta.IsRancherManaged(r.kc.RESTMapper()) {
		projects, err = clustermeta.ListRancherProjects(r.kc)
		if err != nil {
			return nil, err
		}
	}

	result := mgmtapi.ProjectList{
		TypeMeta: metav1.TypeMeta{},
		// ListMeta: nil,
		Items: projects,
	}

	return &result, err
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}
