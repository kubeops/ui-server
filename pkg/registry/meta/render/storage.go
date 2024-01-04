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

package render

import (
	"context"
	"strings"

	"kubeops.dev/ui-server/pkg/graph"

	openvizcs "go.openviz.dev/apimachinery/client/clientset/versioned"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/registry/rest"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	tabledefs "kmodules.xyz/resource-metadata/hub/resourcetabledefinitions"
	"kmodules.xyz/resource-metadata/pkg/tableconvertor"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc client.Client
	oc openvizcs.Interface
	a  authorizer.Authorizer
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client, oc openvizcs.Interface, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc: kc,
		oc: oc,
		a:  a,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindRender)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rsapi.ResourceKindRender)
}

func (r *Storage) New() runtime.Object {
	return &rsapi.Render{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*rsapi.Render)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}
	req := in.Request

	var resp rsapi.RenderResponse
	if req.Block != nil {
		var autoColumns bool
		if len(req.Block.View.Columns) == 0 {
			// copied from: https://github.com/kmodules/resource-metadata/blob/v0.9.9/pkg/layouts/lib.go#L331-L348
			var columns []rsapi.ResourceColumnDefinition
			mapping, err := r.kc.RESTMapper().RESTMapping(schema.GroupKind{Group: req.Block.Ref.Group, Kind: req.Block.Ref.Kind})
			if meta.IsNoMatchError(err) {
				columns = tableconvertor.FilterColumnsWithDefaults(nil, schema.GroupVersionResource{} /*ignore*/, columns, rsapi.List)
			} else if err == nil {
				if rv, ok := tabledefs.LoadDefaultByGVK(mapping.GroupVersionKind); ok {
					columns = rv.Spec.Columns
				}
				columns, err = tabledefs.FlattenColumns(columns)
				if err != nil {
					return nil, err
				}
				columns = tableconvertor.FilterColumnsWithDefaults(r.kc, mapping.Resource, columns, rsapi.List)
			}
			req.Block.View.Columns = columns
			autoColumns = true
		}

		bv, err := graph.RenderPageBlock(r.kc, r.oc, req.Source, req.Block, req.ConvertToTable)
		if err != nil {
			return nil, err
		}
		resp.Block = bv
		if autoColumns {
			req.Block.View.Columns = nil
		}
	} else {
		renderBlocks := sets.New[string]()
		for _, k := range req.RenderBlocks {
			renderBlocks.Insert(string(k))
		}
		rv, err := graph.RenderLayout(
			r.kc,
			r.oc,
			req.Source,
			req.LayoutName, // optional
			req.PageName,   // optional
			req.ConvertToTable,
			renderBlocks,
		)
		if err != nil {
			return nil, err
		}
		resp.View = rv
	}
	in.Response = &resp

	return in, nil
}
