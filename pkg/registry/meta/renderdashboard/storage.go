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

package renderdashboard

import (
	"context"
	"fmt"

	"kubeops.dev/ui-server/pkg/graph"

	openvizcs "go.openviz.dev/apimachinery/client/clientset/versioned"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/resourcedashboards"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc client.Client
	oc openvizcs.Interface
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Creater                  = &Storage{}
)

func NewStorage(kc client.Client, oc openvizcs.Interface) *Storage {
	return &Storage{
		kc: kc,
		oc: oc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindRenderDashboard)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &rsapi.RenderDashboard{}
}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*rsapi.RenderDashboard)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}
	req := in.Request

	rid := req.Resource
	id, err := kmapi.ExtractResourceID(r.kc.RESTMapper(), rid)
	if client.IgnoreNotFound(err) != nil {
		klog.V(3).InfoS(fmt.Sprintf("failed to extract resource id for %+v", rid))
	}
	rid = *id

	var src unstructured.Unstructured
	src.SetGroupVersionKind(rid.GroupVersionKind())
	err = r.kc.Get(context.TODO(), req.Ref.ObjectKey(), &src)
	if err != nil {
		return nil, err
	}

	var rd *rsapi.ResourceDashboard
	if req.Name == "" {
		if rd, err = resourcedashboards.LoadByGVR(r.kc, rid.GroupVersionResource()); err != nil {
			return nil, err
		}
	} else {
		if rd, err = resourcedashboards.LoadByName(r.kc, req.Name); err != nil {
			return nil, err
		}
	}

	dg, err := graph.RenderDashboard(r.kc, r.oc, rd, &src)
	if err != nil {
		return nil, err
	}
	in.Response = &rsapi.RenderDashboardResponse{
		Dashboards: make([]rsapi.DashboardResponse, 0, len(dg.Response.Dashboards)),
	}
	for _, e := range dg.Response.Dashboards {
		conv := rsapi.DashboardResponse{
			Title:  e.Title,
			URL:    e.URL,
			Panels: make([]rsapi.PanelLinkResponse, 0, len(e.Panels)),
		}
		for _, p := range e.Panels {
			conv.Panels = append(conv.Panels, rsapi.PanelLinkResponse{
				Title: p.Title,
				URL:   p.URL,
			})
		}
		in.Response.Dashboards = append(in.Response.Dashboards, conv)
	}
	return in, nil
}