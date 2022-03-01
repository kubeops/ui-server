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

package renderapi

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/registry/rest"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc client.Client
	a  authorizer.Authorizer
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Creater                  = &Storage{}
)

func NewStorage(kc client.Client, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc: kc,
		a:  a,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindRenderAPI)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &rsapi.RenderAPI{}
}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*rsapi.RenderAPI)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}
	req := in.Request

	if req.Selector == nil {
		var out unstructured.Unstructured
		out.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   req.Resource.Group,
			Version: req.Resource.Version,
			Kind:    req.Resource.Kind,
		})
		err := r.kc.Get(context.TODO(), client.ObjectKey{Namespace: req.Ref.Namespace, Name: req.Ref.Name}, &out)
		if err != nil {
			return nil, err
		}
		in.Response = &out
	} else {
		selector, err := metav1.LabelSelectorAsSelector(req.Selector)
		if err != nil {
			return nil, err
		}
		opts := client.ListOptions{
			Namespace:     req.Ref.Namespace,
			LabelSelector: selector,
		}
		var out unstructured.UnstructuredList
		out.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   req.Resource.Group,
			Version: req.Resource.Version,
			Kind:    req.Resource.Kind,
		})
		err = r.kc.List(context.TODO(), &out, &opts)
		if err != nil {
			return nil, err
		}
		in.Response = &out
	}

	return in, nil
}
