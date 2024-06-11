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

package whoami

import (
	"context"
	"strings"

	"kubeops.dev/ui-server/pkg/b3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	identityapi "kmodules.xyz/resource-metadata/apis/identity/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc         client.Client
	bc         *b3.Client
	clusterUID string
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client, bc *b3.Client, clusterUID string) *Storage {
	return &Storage{
		kc:         kc,
		bc:         bc,
		clusterUID: clusterUID,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return identityapi.SchemeGroupVersion.WithKind(identityapi.ResourceKindInboxTokenRequest)
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
		JmapJWTToken:  "your-jmap-token-here",
		AdminJWTToken: "your-admin-token-here",
	}
	return req, nil
}
