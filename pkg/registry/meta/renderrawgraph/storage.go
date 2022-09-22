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

package renderrawgraph

import (
	"context"

	"kubeops.dev/ui-server/pkg/graph"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc client.Client
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
)

func NewStorage(kc client.Client) *Storage {
	return &Storage{
		kc: kc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindRenderRawGraph)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &rsapi.RenderRawGraph{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*rsapi.RenderRawGraph)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}

	var oid kmapi.OID
	if in.Request.Source != nil {
		rid := in.Request.Source.Resource
		if rid.Kind == "" {
			r2, err := kmapi.ExtractResourceID(r.kc.RESTMapper(), in.Request.Source.Resource)
			if err != nil {
				return nil, err
			}
			rid = *r2
		}
		src := kmapi.ObjectID{
			Group:     rid.Group,
			Kind:      rid.Kind,
			Namespace: in.Request.Source.Ref.Namespace,
			Name:      in.Request.Source.Ref.Name,
		}
		oid = src.OID()
	}

	resp, err := graph.Render(oid)
	if err != nil {
		return nil, err
	}
	in.Response = resp
	return in, nil
}
