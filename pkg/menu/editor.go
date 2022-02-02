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

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func LoadResourceEditor(kc client.Client, gvr schema.GroupVersionResource) (*v1alpha1.ResourceEditor, bool) {
	var ed v1alpha1.ResourceEditor
	err := kc.Get(context.TODO(), client.ObjectKey{Name: resourceeditors.DefaultEditorName(gvr)}, &ed)
	if err == nil {
		return &ed, true
	} else if client.IgnoreNotFound(err) != nil {
		klog.V(8).InfoS(fmt.Sprintf("failed to load resource editor for %+v", gvr))
	}
	return resourceeditors.LoadForGVR(gvr)
}
