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
	gourl "net/url"
	"path"
	"sort"

	"github.com/pkg/errors"
	"gomodules.xyz/pointer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/klog/v2"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/menuoutlines"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
	"kubepack.dev/kubepack/pkg/lib"
	chartsapi "kubepack.dev/preset/apis/charts/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RenderGalleryMenu(kc client.Client, disco discovery.ServerResourcesInterface, menuName string) (*v1alpha1.Menu, error) {
	mo, err := menuoutlines.LoadByName(menuName)
	if err != nil {
		return nil, err
	}

	out, err := GenerateMenuItems(kc, disco)
	if err != nil {
		return nil, err
	}

	menu := v1alpha1.Menu{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
			Kind:       v1alpha1.ResourceKindMenu,
		},
		Home:     mo.Home,
		Sections: nil,
	}

	for _, so := range mo.Sections {
		sec := v1alpha1.MenuSection{
			MenuSectionInfo: so.MenuSectionInfo,
		}
		if sec.AutoDiscoverAPIGroup != "" {
			kinds := out[sec.AutoDiscoverAPIGroup]
			for _, item := range kinds {
				sec.Items = append(sec.Items, *item) // variants
			}
		} else {
			items := make([]v1alpha1.MenuItem, 0)
			for _, item := range so.Items {
				mi := v1alpha1.MenuItem{
					Name:       item.Name,
					Path:       item.Path,
					Resource:   nil,
					Missing:    true,
					Required:   item.Required,
					LayoutName: item.LayoutName,
					Icons:      item.Icons,
					Installer:  nil,
				}

				if item.Type != nil {
					if generated, ok := getMenuItem(out, *item.Type); ok {
						mi.Resource = generated.Resource
						mi.Missing = false
						mi.Installer = generated.Installer
						if mi.LayoutName == "" {
							mi.LayoutName = generated.LayoutName
						}
					}
				}

				ed, ok := getEditor(mi.Resource)
				if !ok || ed.Spec.UI == nil || ed.Spec.UI.Options == nil || len(ed.Spec.Variants) == 0 {
					items = append(items, mi)
				} else if mi.Resource != nil {
					gvr := mi.Resource.GroupVersionResource()
					ed, ok := LoadResourceEditor(kc, gvr)
					if !ok {
						return nil, fmt.Errorf("ResourceEditor not defined for %+v", gvr)
					}

					chartRef := ed.Spec.UI.Options
					chrt, err := lib.DefaultRegistry.GetChart(chartRef.URL, chartRef.Name, chartRef.Version)
					if err != nil {
						klog.Fatal(err)
					}

					vpsMap, err := LoadVendorPresets(chrt)
					if err != nil {
						return nil, errors.Wrapf(err, "failed to load vendor presets for chart %+v", chartRef)
					}

					for _, ref := range ed.Spec.Variants {
						if ref.APIGroup == nil {
							ref.APIGroup = pointer.StringP(chartsapi.GroupVersion.Group)
						}
						if ref.Kind != chartsapi.ResourceKindVendorChartPreset && ref.Kind != chartsapi.ResourceKindClusterChartPreset {
							return nil, fmt.Errorf("unknown preset kind %q used in menu item %s", ref.Kind, mi.Name)
						}

						qs := gourl.Values{}
						qs.Set("preset-group", *ref.APIGroup)
						qs.Set("preset-kind", ref.Kind)
						qs.Set("preset-name", ref.Name)
						u := gourl.URL{
							Path:     path.Join(mi.Resource.Group, mi.Resource.Version, mi.Resource.Name),
							RawQuery: qs.Encode(),
						}

						name, err := GetPresetName(kc, chartRef, vpsMap, ref)
						if err != nil {
							return nil, err
						}

						if len(ed.Spec.Variants) == 1 {
							// cp := mi
							mi.Name = name
							mi.Path = u.String()
							mi.Preset = &ref
							items = append(items, mi)
						} else {
							cp := mi
							cp.Name = name
							cp.Path = u.String()
							cp.Preset = &ref
							items = append(items, cp)
						}
					}
				}
			}
			sec.Items = items
		}
		sort.Slice(sec.Items, func(i, j int) bool {
			return sec.Items[i].Name < sec.Items[j].Name
		})

		if len(sec.Items) > 0 {
			menu.Sections = append(menu.Sections, &sec)
		}
	}

	return &menu, nil
}

func getEditor(rid *kmapi.ResourceID) (*v1alpha1.ResourceEditor, bool) {
	if rid == nil {
		return nil, false
	}

	gvr := rid.GroupVersionResource()
	ed, ok := resourceeditors.LoadForGVR(gvr)
	return ed, ok
}
