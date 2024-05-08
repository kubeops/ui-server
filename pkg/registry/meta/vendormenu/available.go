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

package vendormenu

import (
	"context"

	"kubeops.dev/ui-server/pkg/menu"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/discovery"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AvailableStorage struct {
	kc    client.Client
	disco discovery.DiscoveryInterface
}

var (
	_ rest.GroupVersionKindProvider = &AvailableStorage{}
	_ rest.Storage                  = &AvailableStorage{}
	_ rest.Getter                   = &AvailableStorage{}
)

func NewAvailableStorage(kc client.Client, disco discovery.DiscoveryInterface) *AvailableStorage {
	return &AvailableStorage{
		kc:    kc,
		disco: disco,
	}
}

func (r *AvailableStorage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindMenu)
}

func (r *AvailableStorage) New() runtime.Object {
	return &rsapi.Menu{}
}

func (r *AvailableStorage) Destroy() {}

func (r *AvailableStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.available(name)
}

func (r *AvailableStorage) available(name string) (*rsapi.Menu, error) {
	all, err := menu.GenerateCompleteMenu(r.kc, r.disco)
	if err != nil {
		return nil, err
	}
	existing, err := menu.RenderAccordionMenu(r.kc, r.disco, name)
	if err != nil {
		return nil, err
	}
	all.Minus(existing)
	return all, nil
}
