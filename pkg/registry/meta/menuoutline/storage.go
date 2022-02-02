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

package menuoutline

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	cu "kmodules.xyz/client-go/client"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/menuoutlines"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type Storage struct {
	kc        client.Client
	ns        string
	convertor rest.TableConvertor
}

var _ rest.GroupVersionKindProvider = &Storage{}
var _ rest.Scoper = &Storage{}
var _ rest.Lister = &Storage{}
var _ rest.Getter = &Storage{}
var _ rest.CreaterUpdater = &Storage{}

func NewStorage(kc client.Client, ns string) *Storage {
	return &Storage{
		kc: kc,
		ns: ns,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    rsapi.SchemeGroupVersion.Group,
			Resource: rsapi.ResourceMenuOutlines,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindMenuOutline)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &rsapi.MenuOutline{}
}

func configmapName(user user.Info, menu string) string {
	return fmt.Sprintf("%s.%s.%s.%s", rsapi.SchemeGroupVersion.Group, rsapi.ResourceMenuOutline, menu, user.GetUID())
}

//nolint
func getMenuName(user user.Info, cmName string) (string, error) {
	str := strings.TrimSuffix(cmName, "."+user.GetUID())
	idx := strings.LastIndexByte(str, '.')
	if idx == -1 {
		return "", fmt.Errorf("configmap name %s does not match expected menuoutline name format", cmName)
	}
	return str[idx:], nil
}

var keyMenu = "menu"

func extractMenu(cm *core.ConfigMap) (*rsapi.MenuOutline, error) {
	data, ok := cm.Data[keyMenu]
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("ConfigMap %s/%s does not name data[%q]", cm.Namespace, cm.Name, keyMenu))
	}
	var obj rsapi.MenuOutline
	if err := yaml.Unmarshal([]byte(data), &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	u, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	cmName := configmapName(u, name)
	var cm core.ConfigMap
	err := r.kc.Get(ctx, client.ObjectKey{Namespace: r.ns, Name: cmName}, &cm)
	if apierrors.IsNotFound(err) {
		return menuoutlines.LoadByName(name)
	} else if err != nil {
		return nil, err
	}
	return extractMenu(&cm)
}

func (r *Storage) NewList() runtime.Object {
	return &rsapi.MenuOutlineList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	u, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	allMenus := map[string]rsapi.MenuOutline{}
	for _, m := range menuoutlines.List() {
		allMenus[m.Name] = m
	}

	var list core.ConfigMapList
	err := r.kc.List(ctx, &list, client.InNamespace(r.ns), client.MatchingLabels{
		"k8s.io/group":     rsapi.SchemeGroupVersion.Group,
		"k8s.io/kind":      rsapi.ResourceKindMenuOutline,
		"k8s.io/owner-uid": u.GetUID(),
	})
	if apierrors.IsNotFound(err) {
		menuoutlines.List()
		return &rsapi.MenuOutlineList{
			TypeMeta: metav1.TypeMeta{},
			// ListMeta: ,
			Items: menuoutlines.List(),
		}, nil
	} else if err != nil {
		return nil, err
	}

	for _, cm := range list.Items {
		menu, err := extractMenu(&cm)
		if err != nil {
			return nil, err
		}
		cmName := configmapName(u, menu.Name)
		if cmName != cm.Name {
			return nil, apierrors.NewInternalError(fmt.Errorf("ConfigMap %s/%s contains unexpected menu %s", cm.Namespace, cm.Name, menu.Name))
		}
		allMenus[menu.Name] = *menu
	}

	menus := make([]rsapi.MenuOutline, 0, len(allMenus))
	for _, rl := range allMenus {
		menus = append(menus, rl)
	}
	sort.Slice(menus, func(i, j int) bool {
		return menus[i].Name < menus[j].Name
	})
	return &rsapi.MenuOutlineList{
		TypeMeta: metav1.TypeMeta{},
		// ListMeta: ,
		Items: menus,
	}, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	return r.createOrUpdate(ctx, obj)
}

func (r *Storage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	var oldObj rsapi.MenuOutline
	oldObj.Name = name
	newObj, err := objInfo.UpdatedObject(ctx, &oldObj)
	if err != nil {
		return nil, false, err
	}

	result, err := r.createOrUpdate(ctx, newObj)
	return result, err == nil, err
}

func (r *Storage) createOrUpdate(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	u, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	menu := obj.(*rsapi.MenuOutline)
	data, err := yaml.Marshal(menu)
	if err != nil {
		return nil, apierrors.NewInternalError(errors.Wrapf(err, "failed to marshal MenuOutline %s into yaml", menu.Name))
	}

	var cm core.ConfigMap
	cm.Namespace = r.ns
	cm.Name = configmapName(u, menu.Name)
	result, _, err := cu.CreateOrPatch(ctx, r.kc, &cm, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*core.ConfigMap)
		in.Data = map[string]string{
			keyMenu: string(data),
		}
		return in
	})
	return result, err
}
