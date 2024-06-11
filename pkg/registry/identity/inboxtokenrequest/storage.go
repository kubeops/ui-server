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

package inboxtokenrequest

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	_ "k8s.io/klog/v2"
	identityapi "kubeops.dev/ui-server/apis/identity/v1alpha1"
	"kubeops.dev/ui-server/pkg/registry/identity"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"sync"
)

type Storage struct {
	kc         client.Client
	bc         *identity.Client
	clusterUID string
	convertor  rest.TableConvertor

	identity *identityapi.ClusterIdentity
	idError  error
	once     sync.Once
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client, bc *identity.Client, clusterUID string) *Storage {
	return &Storage{
		kc:         kc,
		bc:         bc,
		clusterUID: clusterUID,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    identityapi.GroupName,
			Resource: identityapi.ResourceClusterIdentities,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return identityapi.GroupVersion.WithKind(identityapi.ResourceKindInboxTokenRequest)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(identityapi.ResourceKindInboxTokenRequest)
}

func (r *Storage) New() runtime.Object {
	return &identityapi.InboxTokenRequest{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	req := obj.(*identityapi.InboxTokenRequest)
	req.Response = &identityapi.InboxTokenRequestResponse{
		AdminJWTToken: r.bc.GetToken(),
	}
	return req, nil
}
