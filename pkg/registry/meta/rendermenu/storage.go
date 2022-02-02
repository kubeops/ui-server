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

package rendermenu

import (
	"context"
	"fmt"

	"kubeops.dev/ui-server/pkg/menu"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/discovery"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc    client.Client
	disco discovery.ServerResourcesInterface
}

var _ rest.GroupVersionKindProvider = &Storage{}
var _ rest.Scoper = &Storage{}
var _ rest.Creater = &Storage{}

func NewStorage(kc client.Client, disco discovery.ServerResourcesInterface) *Storage {
	return &Storage{
		kc:    kc,
		disco: disco,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.ResourceKindRenderMenu)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &v1alpha1.RenderMenu{}
}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*v1alpha1.RenderMenu)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}
	req := in.Request

	switch req.Mode {
	case v1alpha1.MenuAccordion:
		if resp, err := menu.RenderAccordionMenu(r.kc, r.disco, req.Menu); err != nil {
			return nil, err
		} else {
			in.Response = resp
		}
	case v1alpha1.MenuGallery:
		if resp, err := menu.RenderGalleryMenu(r.kc, r.disco, req.Menu); err != nil {
			return nil, err
		} else {
			in.Response = resp
		}
	case v1alpha1.MenuDropDown:
		if resp, err := menu.RenderDropDownMenu(r.kc, r.disco, req); err != nil {
			return nil, err
		} else {
			in.Response = resp
		}
	default:
		return nil, apierrors.NewBadRequest(fmt.Sprintf("unknown menu mode %s", req.Mode))
	}
	return in, nil
}
