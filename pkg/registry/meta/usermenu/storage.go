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
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/discovery"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Client
	disco     discovery.ServerResourcesInterface
	ns        string
	convertor rest.TableConvertor
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Lister                   = &Storage{}
	_ rest.Getter                   = &Storage{}
	_ rest.CreaterUpdater           = &Storage{}
	_ rest.GracefulDeleter          = &Storage{}
)

func NewStorage(kc client.Client, disco discovery.ServerResourcesInterface, ns string) *Storage {
	return &Storage{
		kc:    kc,
		disco: disco,
		ns:    ns,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    rsapi.SchemeGroupVersion.Group,
			Resource: rsapi.ResourceMenus,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindMenu)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &rsapi.Menu{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	driver := menu.NewUserMenuDriver(r.kc, r.disco, r.ns, user.GetName())
	return driver.Get(name)
}

func (r *Storage) NewList() runtime.Object {
	return &rsapi.MenuList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	driver := menu.NewUserMenuDriver(r.kc, r.disco, r.ns, user.GetName())
	return driver.List()
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	driver := menu.NewUserMenuDriver(r.kc, r.disco, r.ns, user.GetName())
	return driver.Upsert(obj.(*rsapi.Menu))
}

func (r *Storage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, false, apierrors.NewBadRequest("missing user info")
	}

	driver := menu.NewUserMenuDriver(r.kc, r.disco, r.ns, user.GetName())
	oldObj, err := driver.Get(name)
	if err != nil {
		return nil, false, err
	}
	newObj, err := objInfo.UpdatedObject(ctx, oldObj)
	if err != nil {
		return nil, false, err
	}

	result, err := driver.Upsert(newObj.(*rsapi.Menu))
	return result, err == nil, err
}

func (r *Storage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, false, apierrors.NewBadRequest("missing user info")
	}

	driver := menu.NewUserMenuDriver(r.kc, r.disco, r.ns, user.GetName())
	result, err := driver.Delete(name)
	return result, true, err
}
