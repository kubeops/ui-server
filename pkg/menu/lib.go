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

// "kmodules.xyz/resource-metadata/hub/resourceclasses"

// const (
// 	crdIconSVG = "https://cdn.appscode.com/k8s/icons/apiextensions.k8s.io/customresourcedefinitions.svg"
// )

// func CompleteResourcePanel(r *hub.Registry, namespace resourceclasses.UINamespace) (*v1alpha1.Menu, error) {
// 	return createResourcePanel(r, namespace, true)
// }

// func DefaultResourcePanel(r *hub.Registry, namespace resourceclasses.UINamespace) (*v1alpha1.Menu, error) {
// 	return createResourcePanel(r, namespace, false)
// }

// func createResourcePanel(r *hub.Registry, namespace resourceclasses.UINamespace, keepOfficialTypes bool) (*v1alpha1.Menu, error) {
// 	sections := make(map[string]*v1alpha1.MenuSection)
// 	existingGRs := map[schema.GroupResource]bool{}

// 	// first add the known required sections
// 	for group, rc := range resourceclasses.KnownClasses[namespace] {
// 		if !rc.IsRequired() && "Helm 3" != rc.Name {
// 			continue
// 		}

// 		section := &v1alpha1.MenuSection{
// 			MenuSectionInfo: v1alpha1.MenuSectionInfo{
// 				Name:  rc.Name,
// 				Icons: rc.Spec.ResourceClassInfo.Icons,
// 			},
// 			// Weight:            rc.Spec.Weight,
// 		}
// 		for _, entry := range rc.Spec.Items {
// 			pe := v1alpha1.MenuItem{
// 				Name:     entry.Name,
// 				Path:     entry.Path,
// 				Required: entry.Required,
// 				Icons:    entry.Icons,
// 				// Namespaced: rc.Name == "Helm 3",
// 				LayoutName: entry.LayoutName,
// 			}
// 			if entry.Type != nil {
// 				gvr, ok := r.FindGVR(entry.Type, keepOfficialTypes)
// 				if !ok {
// 					continue
// 				}
// 				pe.Resource = &kmapi.ResourceID{
// 					Group:   gvr.Group,
// 					Version: gvr.Version,
// 					Name:    gvr.Resource,
// 				}
// 				existingGRs[gvr.GroupResource()] = true
// 				if rd, err := r.LoadByGVR(gvr); err == nil {
// 					pe.Resource = &rd.Spec.Resource
// 					// pe.Namespaced = rd.Spec.Resource.Scope == kmapi.NamespaceScoped
// 					pe.Icons = rd.Spec.Icons
// 					pe.Missing = r.Missing(gvr)
// 					// pe.Installer = rd.Spec.Installer
// 					if pe.LayoutName == "" {
// 						pe.LayoutName = resourceoutlines.DefaultLayoutName(rd.Spec.Resource.GroupVersionResource())
// 					}
// 				}
// 			}
// 			section.Items = append(section.Items, pe)
// 		}
// 		sections[group] = section
// 	}

// 	// now, auto discover sections from registry
// 	r.Visit(func(_ string, rd *v1alpha1.ResourceDescriptor) {
// 		if !keepOfficialTypes && v1alpha1.IsOfficialType(rd.Spec.Resource.Group) {
// 			return // skip k8s.io api groups
// 		}

// 		gvr := rd.Spec.Resource.GroupVersionResource()
// 		if _, found := existingGRs[gvr.GroupResource()]; found {
// 			return
// 		}

// 		section, found := sections[rd.Spec.Resource.Group]
// 		if !found {
// 			if rc, found := resourceclasses.KnownClasses[namespace][rd.Spec.Resource.Group]; found {
// 				//w := math.MaxInt16
// 				//if rc.Spec.Weight > 0 {
// 				//	w = rc.Spec.Weight
// 				//}
// 				section = &v1alpha1.MenuSection{
// 					MenuSectionInfo: v1alpha1.MenuSectionInfo{
// 						Name:  rc.Name,
// 						Icons: rc.Spec.ResourceClassInfo.Icons,
// 					},
// 					// Weight:            w,
// 				}
// 			} else {
// 				// unknown api group, so use CRD icon
// 				name := resourceclasses.ResourceClassName(rd.Spec.Resource.Group)
// 				section = &v1alpha1.MenuSection{
// 					MenuSectionInfo: v1alpha1.MenuSectionInfo{
// 						Name: name,
// 					},
// 					//ResourceClassInfo: v1alpha1.ResourceClassInfo{
// 					//	APIGroup: rd.Spec.Resource.Group,
// 					//},
// 					// Weight: math.MaxInt16,
// 				}
// 			}
// 			sections[rd.Spec.Resource.Group] = section
// 		}

// 		section.Items = append(section.Items, v1alpha1.MenuItem{
// 			Name:     rd.Spec.Resource.Kind,
// 			Resource: &rd.Spec.Resource,
// 			Icons:    rd.Spec.Icons,
// 			// Namespaced: rd.Spec.Resource.Scope == kmapi.NamespaceScoped,
// 			Missing: r.Missing(gvr),
// 			// Installer:  rd.Spec.Installer,
// 			LayoutName: resourceoutlines.DefaultLayoutName(rd.Spec.Resource.GroupVersionResource()),
// 		})
// 		existingGRs[gvr.GroupResource()] = true
// 	})

// 	return toPanel(sections)
// }

// func toPanel(in map[string]*v1alpha1.MenuSection) (*v1alpha1.Menu, error) {
// 	sections := make([]*v1alpha1.MenuSection, 0, len(in))

// 	for key, section := range in {
// 		if !strings.HasSuffix(key, ".local") {
// 			sort.Slice(section.Items, func(i, j int) bool {
// 				return section.Items[i].Name < section.Items[j].Name
// 			})
// 		}

// 		// Set icon for sections missing icon
// 		if len(section.Icons) == 0 {
// 			// TODO: Use a different icon for section
// 			section.Icons = []v1alpha1.ImageSpec{
// 				{
// 					Source: crdIconSVG,
// 					Type:   "image/svg+xml",
// 				},
// 			}
// 		}
// 		// set icons for entries missing icon
// 		for i := range section.Items {
// 			if len(section.Items[i].Icons) == 0 {
// 				section.Items[i].Icons = []v1alpha1.ImageSpec{
// 					{
// 						Source: crdIconSVG,
// 						Type:   "image/svg+xml",
// 					},
// 				}
// 			}
// 		}

// 		sections = append(sections, section)
// 	}

// 	//sort.Slice(sections, func(i, j int) bool {
// 	//	if sections[i].Weight == sections[j].Weight {
// 	//		return sections[i].Name < sections[j].Name
// 	//	}
// 	//	return sections[i].Weight < sections[j].Weight
// 	//})

// 	return &v1alpha1.Menu{Sections: sections}, nil
// }
