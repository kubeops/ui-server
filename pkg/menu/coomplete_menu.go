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
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub"
	"kmodules.xyz/resource-metadata/hub/menuoutlines"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var defaultIcons = []v1alpha1.ImageSpec{
	{
		Source: hub.CRDIconSVG,
		Type:   "image/svg+xml",
	},
}

func GenerateCompleteMenu(kc client.Client, disco discovery.ServerResourcesInterface) (*v1alpha1.Menu, error) {
	sectionIcons := map[string][]v1alpha1.ImageSpec{}
	for _, m := range menuoutlines.List() {
		for _, sec := range m.Sections {
			if sec.AutoDiscoverAPIGroup != "" {
				sectionIcons[sec.AutoDiscoverAPIGroup] = sec.Icons
			}
		}
	}

	out, err := GenerateMenuItems(kc, disco)
	if err != nil {
		return nil, err
	}

	sections := make([]*v1alpha1.MenuSection, 0, len(out))
	for group, kinds := range out {
		sec := v1alpha1.MenuSection{
			MenuSectionInfo: v1alpha1.MenuSectionInfo{
				Name: menuoutlines.MenuSectionName(group),
			},
		}
		if icons, ok := sectionIcons[group]; ok {
			sec.Icons = icons
		} else {
			sec.Icons = defaultIcons
		}

		for _, item := range kinds {
			sec.Items = append(sec.Items, *item) // variants
		}
		sort.Slice(sec.Items, func(i, j int) bool {
			return sec.Items[i].Name < sec.Items[j].Name
		})

		if len(sec.Items) > 0 {
			sections = append(sections, &sec)
		}
	}

	sort.Slice(sections, func(i, j int) bool {
		return sections[i].Name < sections[j].Name
	})

	return &v1alpha1.Menu{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
			Kind:       v1alpha1.ResourceKindMenuOutline,
		},
		Sections: sections,
	}, nil
}
