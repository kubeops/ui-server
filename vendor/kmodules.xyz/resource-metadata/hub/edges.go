/*
Copyright AppsCode Inc. and Contributors

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

package hub

import (
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"

	"k8s.io/apimachinery/pkg/util/sets"
)

func ListEdgeLabels() []v1alpha1.EdgeLabel {
	labels := sets.NewString()
	reg := NewRegistryOfKnownResources()
	reg.Visit(func(key string, rd *v1alpha1.ResourceDescriptor) {
		for _, c := range rd.Spec.Connections {
			for _, lbl := range c.Labels {
				labels.Insert(string(lbl))
			}
		}
	})

	result := make([]v1alpha1.EdgeLabel, 0, len(labels))
	for lbl := range labels {
		result = append(result, v1alpha1.EdgeLabel(lbl))
	}
	return result
}
