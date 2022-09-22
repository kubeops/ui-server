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

	identityv1alpha1 "kubeops.dev/ui-server/apis/identity/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

type Storage struct{}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
)

func NewStorage() *Storage {
	return &Storage{}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return identityv1alpha1.GroupVersion.WithKind(identityv1alpha1.ResourceKindWhoAmI)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &identityv1alpha1.WhoAmI{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	user, ok := request.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}
	req := obj.(*identityv1alpha1.WhoAmI)

	extra := make(map[string]identityv1alpha1.ExtraValue)
	for k, v := range user.GetExtra() {
		extra[k] = v
	}
	req.Response = &identityv1alpha1.WhoAmIResponse{
		User: &identityv1alpha1.UserInfo{
			Username: user.GetName(),
			UID:      user.GetUID(),
			Groups:   user.GetGroups(),
			Extra:    extra,
		},
	}
	return req, nil
}
