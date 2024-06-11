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

package clusteridentity

import (
	"context"
	"strings"

	"kubeops.dev/ui-server/pkg/b3"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	identityapi "kmodules.xyz/resource-metadata/apis/identity/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Client
	bc        *b3.Client
	convertor rest.TableConvertor
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Lister                   = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client, bc *b3.Client) *Storage {
	return &Storage{
		kc: kc,
		bc: bc,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    identityapi.GroupName,
			Resource: identityapi.ResourceClusterIdentities,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return identityapi.SchemeGroupVersion.WithKind(identityapi.ResourceKindClusterIdentity)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(identityapi.ResourceKindClusterIdentity)
}

func (r *Storage) New() runtime.Object {
	return &identityapi.ClusterIdentity{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	if name != b3.SelfName {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: identityapi.GroupName, Resource: identityapi.ResourceClusterIdentities}, name)
	}
	return r.bc.GetIdentity()
}

// Lister
func (r *Storage) NewList() runtime.Object {
	return &identityapi.ClusterIdentityList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	id, err := r.bc.GetIdentity()
	if err != nil {
		return nil, err
	}
	result := identityapi.ClusterIdentityList{
		TypeMeta: metav1.TypeMeta{},
		Items: []identityapi.ClusterIdentity{
			*id,
		},
	}
	return &result, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}
