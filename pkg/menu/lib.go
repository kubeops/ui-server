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
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
	"kmodules.xyz/resource-metadata/hub/resourceoutlines"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RenderMenu(driver *UserMenuDriver, req *rsapi.RenderMenuRequest) (*rsapi.Menu, error) {
	switch req.Mode {
	case rsapi.MenuAccordion:
		return driver.Get(req.Menu)
	case rsapi.MenuGallery:
		return GetGalleryMenu(driver, req)
	default:
		return nil, apierrors.NewBadRequest(fmt.Sprintf("unknown menu mode %s", req.Mode))
	}
}

func GenerateMenuItems(kc client.Client, disco discovery.ServerResourcesInterface) (map[string]map[string]*rsapi.MenuItem, error) {
	rsLists, err := disco.ServerPreferredResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return nil, err
	}

	// [group][Kind] => MenuItem
	out := map[string]map[string]*rsapi.MenuItem{}
	for _, rsList := range rsLists {
		gv, err := schema.ParseGroupVersion(rsList.GroupVersion)
		if err != nil {
			return nil, err
		}

		for _, rs := range rsList.APIResources {
			// skip sub resource
			if strings.ContainsRune(rs.Name, '/') {
				continue
			}

			// if resource can't be listed or read (get) or only view type skip it
			verbs := sets.NewString(rs.Verbs...)
			if !verbs.HasAll("list", "get") {
				continue
			}

			scope := kmapi.ClusterScoped
			if rs.Namespaced {
				scope = kmapi.NamespaceScoped
			}
			rid := kmapi.ResourceID{
				Group:   gv.Group,
				Version: gv.Version,
				Name:    rs.Name,
				Kind:    rs.Kind,
				Scope:   scope,
			}
			gvr := rid.GroupVersionResource()

			me := rsapi.MenuItem{
				Name:       rid.Kind,
				Path:       "",
				Resource:   &rid,
				Missing:    false,
				Required:   false,
				LayoutName: resourceoutlines.DefaultLayoutName(gvr),
				// Icons:    rd.Spec.Icons,
				// Installer:  rd.Spec.Installer,
			}
			if ed, ok := resourceeditors.LoadByGVR(kc, gvr); ok {
				me.Icons = ed.Spec.Icons
				me.Installer = ed.Spec.Installer

				me.AvailableVariants = len(ed.Spec.Variants)
				if me.AvailableVariants == 1 {
					me.Variant = ed.Spec.Variants[0].Name
				}
			}

			if _, ok := out[gv.Group]; !ok {
				out[gv.Group] = map[string]*rsapi.MenuItem{}
			}
			out[gv.Group][rs.Kind] = &me // variants
		}
	}

	return out, nil
}

func getMenuItem(out map[string]map[string]*rsapi.MenuItem, gk metav1.GroupKind) (*rsapi.MenuItem, bool) {
	m, ok := out[gk.Group]
	if !ok {
		return nil, false
	}
	item, ok := m[gk.Kind]
	return item, ok
}
