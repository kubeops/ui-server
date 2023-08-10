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

package graph

import (
	"kubeops.dev/ui-server/pkg/shared"

	"k8s.io/apimachinery/pkg/runtime/schema"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/resourcedescriptors"
	"kmodules.xyz/resource-metadata/pkg/tableconvertor"
)

func RenderExec(src, target *schema.GroupVersionResource) tableconvertor.ResourceExecFunc {
	var rdSrc, rdTarget *rsapi.ResourceDescriptor
	if src != nil {
		rdSrc, _ = resourcedescriptors.LoadByGVR(*src)
	}
	if target != nil {
		rdTarget, _ = resourcedescriptors.LoadByGVR(*target)
	}
	return func() []rsapi.ResourceExec {
		if rdTarget != nil && len(rdTarget.Spec.Exec) > 0 {
			return rdTarget.Spec.Exec
		}
		if shared.IsPod(*target) && rdSrc != nil && len(rdSrc.Spec.Exec) > 0 {
			in := rdSrc.Spec.Exec[0]
			out := rsapi.ResourceExec{
				Alias:               "",
				If:                  nil,
				ServiceNameTemplate: "",
				Container:           in.Container,
				Command:             in.Command,
				Help:                in.Help,
			}
			return []rsapi.ResourceExec{out}
		}
		return nil
	}
}
