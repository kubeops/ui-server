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
	"sort"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	mu "kmodules.xyz/client-go/meta"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetGalleryMenu(driver *UserMenuDriver, opts *rsapi.RenderMenuRequest) (*rsapi.Menu, error) {
	menu, err := driver.Get(opts.Menu)
	if err != nil {
		return nil, err
	}
	return RenderGalleryMenu(driver.GetClient(), driver.disco, menu, opts)
}

func RenderGalleryMenu(kc client.Client, disco discovery.DiscoveryInterface, in *rsapi.Menu, opts *rsapi.RenderMenuRequest) (*rsapi.Menu, error) {
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disco))

	out := rsapi.Menu{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rsapi.SchemeGroupVersion.String(),
			Kind:       rsapi.ResourceKindMenu,
		},
		ObjectMeta: in.ObjectMeta,
		Spec: rsapi.MenuSpec{
			Mode: rsapi.MenuGallery,
			Home: in.Spec.Home,
		},
	}

	for _, so := range in.Spec.Sections {
		if opts.Section != nil && so.Name != *opts.Section {
			continue
		}

		items := make([]rsapi.MenuItem, 0)
		for _, item := range so.Items {
			mi := rsapi.MenuItem{
				Name:        item.Name,
				Path:        item.Path,
				Resource:    item.Resource,
				Missing:     item.Missing,
				Required:    item.Required,
				LayoutName:  item.LayoutName,
				Icons:       item.Icons,
				Installer:   item.Installer,
				FeatureMode: item.FeatureMode,
			}

			if mi.Resource != nil &&
				opts.Type != nil &&
				(opts.Type.Group != mi.Resource.Group || opts.Type.Kind != mi.Resource.Kind) {
				continue
			}
			if mi.Resource.Kind != "" && !gkExists(mapper, item.Resource.GroupKind()) {
				mi.Missing = true
			}

			ed, err := resourceeditors.LoadByResourceID(kc, mi.Resource)
			if err != nil || ed.Spec.UI == nil || ed.Spec.UI.Options == nil || len(ed.Spec.Variants) == 0 {
				items = append(items, mi)
			} else {
				chartRef := ed.Spec.UI.Options
				if chartRef.SourceRef.Namespace == "" {
					chartRef.SourceRef.Namespace = mu.PodNamespace()
				}

				for _, ref := range ed.Spec.Variants {
					cp := mi
					if len(ed.Spec.Variants) > 1 && ref.Title == "" {
						return nil, fmt.Errorf("resource editor for %+v and variant %s is missing title", ed.Spec.Resource.GroupKind(), ref.Name)
					}
					if ref.Title != "" {
						cp.Name = ref.Title
					}
					// cp.Path = u.String()
					cp.Variant = ref.Name
					if len(ref.Icons) > 0 {
						cp.Icons = ref.Icons
					}
					items = append(items, cp)
				}
			}
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].Name < items[j].Name
		})

		if len(items) > 0 {
			out.Spec.Sections = append(out.Spec.Sections, &rsapi.MenuSection{
				MenuSectionInfo: so.MenuSectionInfo,
				Items:           items,
			})
		}
	}

	return &out, nil
}

func gkExists(mapper meta.RESTMapper, gk schema.GroupKind) bool {
	if _, err := mapper.RESTMappings(schema.GroupKind{
		Group: gk.Group,
		Kind:  gk.Kind,
	}); err == nil {
		return true
	}
	return false
}
