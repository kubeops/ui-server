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

package usermenu

import (
	"context"

	"kubeops.dev/ui-server/pkg/menu"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/discovery"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AvailableStorage struct {
	kc    client.Client
	disco discovery.ServerResourcesInterface
	ns    string
}

var _ rest.GroupVersionKindProvider = &AvailableStorage{}
var _ rest.Getter = &AvailableStorage{}

func NewAvailableStorage(kc client.Client, disco discovery.ServerResourcesInterface, ns string) *AvailableStorage {
	return &AvailableStorage{
		kc:    kc,
		disco: disco,
		ns:    ns,
	}
}

func (r *AvailableStorage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindMenu)
}

func (r *AvailableStorage) New() runtime.Object {
	return &rsapi.Menu{}
}

func (r *AvailableStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	driver := menu.NewUserMenuDriver(r.kc, r.disco, r.ns, user.GetName())
	return driver.Available(name)
}
