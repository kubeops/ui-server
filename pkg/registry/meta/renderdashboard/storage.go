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
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"kubeops.dev/ui-server/pkg/graph"
	"kubeops.dev/ui-server/pkg/shared"

	"github.com/Masterminds/sprig/v3"
	"github.com/pkg/errors"
	openvizauipi "go.openviz.dev/apimachinery/apis/ui/v1alpha1"
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

	if rd.Spec.Provider != rsapi.DashboardProviderGrafana {
		return nil, fmt.Errorf("dashboard %s uses unsupported provider %q", rd.Name, rd.Spec.Provider)
	}

	buf := shared.BufferPool.Get().(*bytes.Buffer)
	defer shared.BufferPool.Put(buf)

	dg := &openvizauipi.DashboardGroup{
		Request: &openvizauipi.DashboardGroupRequest{
			Dashboards: make([]openvizauipi.DashboardRequest, 0, len(rd.Spec.Dashboards)),
		},
	}
	for _, d := range rd.Spec.Dashboards {
		cond := true
		if d.If.Condition != "" {
			result, err := renderTemplate(d.If.Condition, src.UnstructuredContent(), buf)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to check condition for dashboard with title %s", d.Title)
			}
			result = strings.TrimSpace(result)
			cond = strings.EqualFold(result, "true")
		} else if d.If.Connected != nil {
			_, targets, err := graph.ExecRawQuery(r.kc, kmapi.NewObjectID(&src).OID(), *d.If.Connected)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to check connection for dashboard with title %s", d.Title)
			}
			cond = len(targets) > 0
		}
		if !cond {
			continue
		}

		out := openvizauipi.DashboardRequest{
			DashboardRef: openvizauipi.DashboardRef{
				Title: d.Title,
			},
			Vars:   make([]openvizauipi.DashboardVar, 0, len(d.Vars)),
			Panels: nil,
		}
		for _, v := range d.Vars {
			if v.Type != rsapi.DashboardVarTypeTarget {
				val, err := renderTemplate(v.Value, src.UnstructuredContent(), buf)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to render the value of variable %q in dashboard with title %s", v.Name, d.Title)
				}
				out.Vars = append(out.Vars, openvizauipi.DashboardVar{
					Name:  v.Name,
					Value: val,
					Type:  openvizauipi.DashboardVarTypeSource,
				})
			}
		}

		dg.Request.Dashboards = append(dg.Request.Dashboards, out)
	}
	dg, err = r.oc.UiV1alpha1().DashboardGroups().Create(context.TODO(), dg, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	in.Response = &rsapi.RenderDashboardResponse{
		Dashboards: make([]rsapi.DashboardResponse, 0, len(dg.Response.Dashboards)),
	}
	for _, e := range dg.Response.Dashboards {
		conv := rsapi.DashboardResponse{
			Title:  e.Title,
			Link:   e.Link,
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

func renderTemplate(text string, data interface{}, buf *bytes.Buffer) (string, error) {
	if !strings.Contains(text, "{{") {
		return text, nil
	}

	tpl, err := template.New("").Funcs(sprig.TxtFuncMap()).Parse(text)
	if err != nil {
		return "", errors.Wrapf(err, "falied to parse template %s", text)
	}
	// Do nothing and continue execution.
	// If printed, the result of the index operation is the string "<no value>".
	// We mitigate that later.
	tpl.Option("missingkey=default")
	buf.Reset()
	err = tpl.Execute(buf, data)
	if err != nil {
		return "", errors.Wrapf(err, "falied to render template %s", text)
	}
	return strings.ReplaceAll(buf.String(), "<no value>", ""), nil
}
