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

package menu

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/zeebo/xxh3"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	cu "kmodules.xyz/client-go/client"
	"kmodules.xyz/client-go/meta"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/menuoutlines"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type UserMenuDriver struct {
	kc    client.Client
	disco discovery.ServerResourcesInterface
	ns    string
	user  string
}

func NewUserMenuDriver(kc client.Client, disco discovery.ServerResourcesInterface, ns, user string) *UserMenuDriver {
	return &UserMenuDriver{
		kc:    kc,
		disco: disco,
		ns:    ns,
		user:  user,
	}
}

func configmapName(user, menu string) string {
	// use core.appscode.com.menu.$menu.$user
	return fmt.Sprintf("appscode.com.%s.v1.%s.%d" /*rsapi.SchemeGroupVersion.Group,*/, rsapi.ResourceMenu, menu, hashUser(user))
}

func hashUser(user string) uint64 {
	h := xxh3.New()
	if _, err := h.WriteString(user); err != nil {
		panic(errors.Wrapf(err, "failed to hash user %s", user))
	}
	return h.Sum64()
}

// nolint
func getMenuName(user string, cmName string) (string, error) {
	str := strings.TrimSuffix(cmName, fmt.Sprintf(".%d", hashUser(user)))
	idx := strings.LastIndexByte(str, '.')
	if idx == -1 {
		return "", fmt.Errorf("configmap name %s does not match expected menuoutline name format", cmName)
	}
	return str[idx:], nil
}

const (
	keyMenu     = "menu"
	keyUsername = "username"
)

func extractMenu(cm *core.ConfigMap) (*rsapi.Menu, error) {
	data, ok := cm.Data[keyMenu]
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("ConfigMap %s/%s does not name data[%q]", cm.Namespace, cm.Name, keyMenu))
	}
	var obj rsapi.Menu
	if err := yaml.Unmarshal([]byte(data), &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

func updateMenuVariantsInfo(kc client.Client, in *rsapi.Menu) *rsapi.Menu {
	for _, s := range in.Spec.Sections {
		s.Items = updateMenuItemVariantsInfo(kc, s.Items)
	}
	return in
}

func updateMenuItemVariantsInfo(kc client.Client, in []rsapi.MenuItem) []rsapi.MenuItem {
	for _, item := range in {
		if item.Resource != nil {
			gvr := item.Resource.GroupVersionResource()
			if ed, ok := resourceeditors.LoadByGVR(kc, gvr); ok {
				item.AvailableVariants = len(ed.Spec.Variants)
				if item.AvailableVariants == 1 {
					item.Preset = &ed.Spec.Variants[0].TypedLocalObjectReference
				}
				if len(item.Items) > 0 {
					item.Items = updateMenuItemVariantsInfo(kc, item.Items)
				}
			}
		}
	}
	return in
}

func (r *UserMenuDriver) GetClient() client.Client {
	return r.kc
}

func (r *UserMenuDriver) GetDiscoveryClient() discovery.ServerResourcesInterface {
	return r.disco
}

func (r *UserMenuDriver) Get(menu string) (*rsapi.Menu, error) {
	cmName := configmapName(r.user, menu)
	var cm core.ConfigMap
	err := r.kc.Get(context.TODO(), client.ObjectKey{Namespace: r.ns, Name: cmName}, &cm)
	if apierrors.IsNotFound(err) {
		return RenderAccordionMenu(r.kc, r.disco, menu)
	} else if err != nil {
		return nil, err
	}
	m, err := extractMenu(&cm)
	if err != nil {
		return nil, err
	}
	return updateMenuVariantsInfo(r.kc, m), nil
}

func (r *UserMenuDriver) List() (*rsapi.MenuList, error) {
	var list core.ConfigMapList
	err := r.kc.List(context.TODO(), &list, client.InNamespace(r.ns), client.MatchingLabels{
		"k8s.io/group": rsapi.SchemeGroupVersion.Group,
		"k8s.io/kind":  rsapi.ResourceKindMenu,
	})
	if apierrors.IsNotFound(err) {
		names := menuoutlines.Names()

		menus := make([]rsapi.Menu, 0, len(names))
		for _, name := range names {
			if menu, err := RenderAccordionMenu(r.kc, r.disco, name); err != nil {
				return nil, err
			} else {
				menus = append(menus, *menu)
			}
		}
		return &rsapi.MenuList{Items: menus}, nil
	} else if err != nil {
		return nil, err
	}

	allMenus := map[string]rsapi.Menu{}
	for _, cm := range list.Items {
		if v, ok := cm.Data[keyUsername]; !ok || v != r.user {
			continue
		}

		menu, err := extractMenu(&cm)
		if err != nil {
			return nil, err
		}
		menu = updateMenuVariantsInfo(r.kc, menu)
		cmName := configmapName(r.user, menu.Name)
		if cmName != cm.Name {
			return nil, apierrors.NewInternalError(fmt.Errorf("ConfigMap %s/%s contains unexpected menu %s", cm.Namespace, cm.Name, menu.Name))
		}
		allMenus[menu.Name] = *menu
	}

	for _, name := range menuoutlines.Names() {
		if _, ok := allMenus[name]; !ok {
			if menu, err := RenderAccordionMenu(r.kc, r.disco, name); err != nil {
				return nil, err
			} else {
				allMenus[name] = *menu
			}
		}
	}

	menus := make([]rsapi.Menu, 0, len(allMenus))
	for _, rl := range allMenus {
		menus = append(menus, rl)
	}
	sort.Slice(menus, func(i, j int) bool {
		return menus[i].Name < menus[j].Name
	})
	return &rsapi.MenuList{Items: menus}, nil
}

func (r *UserMenuDriver) Upsert(menu *rsapi.Menu) (*rsapi.Menu, error) {
	data, err := yaml.Marshal(menu)
	if err != nil {
		return nil, apierrors.NewInternalError(errors.Wrapf(err, "failed to marshal Menu %s into yaml", menu.Name))
	}

	var cm core.ConfigMap
	cm.Namespace = r.ns
	cm.Name = configmapName(r.user, menu.Name)
	_, err = cu.CreateOrPatch(context.TODO(), r.kc, &cm, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*core.ConfigMap)
		in.Labels = meta.OverwriteKeys(in.Labels, map[string]string{
			"k8s.io/group": rsapi.SchemeGroupVersion.Group,
			"k8s.io/kind":  rsapi.ResourceKindMenu,
		})
		// r.user contains invalid chars for label value, so stored in data
		in.Data = map[string]string{
			keyUsername: r.user,
			keyMenu:     string(data),
		}
		return in
	})
	return menu, err
}

func (r *UserMenuDriver) Available(menu string) (*rsapi.Menu, error) {
	all, err := GenerateCompleteMenu(r.kc, r.disco)
	if err != nil {
		return nil, err
	}
	existing, err := r.Get(menu)
	if err != nil {
		return nil, err
	}
	all.Minus(existing)
	return all, nil
}

func (r *UserMenuDriver) Delete(menu string) (*rsapi.Menu, error) {
	cmName := configmapName(r.user, menu)
	m, err := r.Get(menu)
	if err != nil {
		return nil, err
	}

	var cm core.ConfigMap
	cm.Namespace = r.ns
	cm.Name = cmName
	err = r.kc.Delete(context.TODO(), &cm)
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}
	return m, nil
}
