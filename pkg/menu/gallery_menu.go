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
	"k8s.io/klog/v2"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
	"kubepack.dev/kubepack/pkg/lib"
	"kubepack.dev/lib-helm/pkg/values"
	chartsapi "kubepack.dev/preset/apis/charts/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetGalleryMenu(driver *UserMenuDriver, opts *rsapi.RenderMenuRequest) (*rsapi.Menu, error) {
	menu, err := driver.Get(opts.Menu)
	if err != nil {
		return nil, err
	}
	return RenderGalleryMenu(driver.GetClient(), menu, opts)
}

func RenderGalleryMenu(kc client.Client, in *rsapi.Menu, opts *rsapi.RenderMenuRequest) (*rsapi.Menu, error) {
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
				Name:       item.Name,
				Path:       item.Path,
				Resource:   item.Resource,
				Missing:    item.Missing,
				Required:   item.Required,
				LayoutName: item.LayoutName,
				Icons:      item.Icons,
				Installer:  item.Installer,
			}

			if mi.Resource != nil &&
				opts.Type != nil &&
				(opts.Type.Group != mi.Resource.Group || opts.Type.Kind != mi.Resource.Kind) {
				continue
			}

			ed, ok := resourceeditors.LoadByResourceID(kc, mi.Resource)
			if !ok || ed.Spec.UI == nil || ed.Spec.UI.Options == nil || len(ed.Spec.Variants) == 0 {
				items = append(items, mi)
			} else if mi.Resource != nil {
				gvr := mi.Resource.GroupVersionResource()
				ed, ok := resourceeditors.LoadByGVR(kc, gvr)
				if !ok {
					return nil, fmt.Errorf("ResourceEditor not defined for %+v", gvr)
				}

				chartRef := ed.Spec.UI.Options
				chrt, err := lib.DefaultRegistry.GetChart(chartRef.URL, chartRef.Name, chartRef.Version)
				if err != nil {
					klog.Fatal(err)
				}

				vpsMap, err := values.LoadVendorPresets(chrt.Chart)
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
					qs.Set("presetGroup", *ref.APIGroup)
					qs.Set("presetKind", ref.Kind)
					qs.Set("presetName", ref.Name)
					u := gourl.URL{
						Path:     path.Join(mi.Resource.Group, mi.Resource.Version, mi.Resource.Name),
						RawQuery: qs.Encode(),
					}

					name, err := GetPresetName(kc, chartRef, vpsMap, ref.TypedLocalObjectReference)
					if err != nil {
						return nil, err
					}

					if len(ed.Spec.Variants) == 1 {
						// cp := mi
						mi.Name = name
						mi.Path = u.String()
						mi.Preset = &ref.TypedLocalObjectReference
						mi.Icons = ref.Icons
						items = append(items, mi)
					} else {
						cp := mi
						cp.Name = name
						cp.Path = u.String()
						cp.Preset = &ref.TypedLocalObjectReference
						mi.Icons = ref.Icons
						items = append(items, cp)
					}
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
