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
	"bytes"
	"context"
	"fmt"
	"strings"

	"kubeops.dev/ui-server/pkg/shared"

	"github.com/pkg/errors"
	openvizauipi "go.openviz.dev/apimachinery/apis/ui/v1alpha1"
	openvizcs "go.openviz.dev/apimachinery/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kmapi "kmodules.xyz/client-go/api/v1"
	sharedapi "kmodules.xyz/resource-metadata/apis/shared"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	"kmodules.xyz/resource-metadata/hub/resourcedashboards"
	"kmodules.xyz/resource-metadata/pkg/tableconvertor"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func renderDashboard(kc client.Client, oc openvizcs.Interface, srcObj *unstructured.Unstructured) tableconvertor.DashboardRendererFunc {
	return func(name string) (*uiapi.ResourceDashboard, string, error) {
		rd, err := resourcedashboards.LoadByName(kc, name)
		if err != nil {
			return nil, "", err
		}
		if rd.Spec.Provider != uiapi.DashboardProviderGrafana {
			return nil, "", fmt.Errorf("unsupported provider %q for dashbaord %s", rd.Spec.Provider, name)
		}
		if len(rd.Spec.Dashboards) == 0 {
			return nil, "", fmt.Errorf("no dashboard configured for %s", name)
		}
		if len(rd.Spec.Dashboards) > 1 {
			return nil, "", fmt.Errorf("multiple dashboards configured for %s", name)
		}
		dg, err := RenderDashboard(kc, oc, rd, srcObj)
		if err != nil {
			return nil, "", err
		}
		return rd, dg.Response.Dashboards[0].URL, nil
	}
}

func RenderDashboard(kc client.Client, oc openvizcs.Interface, rd *uiapi.ResourceDashboard, src *unstructured.Unstructured) (*openvizauipi.DashboardGroup, error) {
	if rd.Spec.Provider != uiapi.DashboardProviderGrafana {
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
		if d.If != nil {
			if d.If.Condition != "" {
				result, err := shared.RenderTemplate(d.If.Condition, src.UnstructuredContent(), buf)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to check condition for dashboard with title %s", d.Title)
				}
				result = strings.TrimSpace(result)
				cond = strings.EqualFold(result, "true")
			} else if d.If.Connected != nil {
				_, targets, err := ExecRawQuery(kc, kmapi.NewObjectID(src).OID(), *d.If.Connected)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to check connection for dashboard with title %s", d.Title)
				}
				cond = len(targets) > 0
			}
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
			if v.Type != sharedapi.DashboardVarTypeTarget {
				val, err := shared.RenderTemplate(v.Value, src.UnstructuredContent(), buf)
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
	return oc.UiV1alpha1().DashboardGroups().Create(context.TODO(), dg, metav1.CreateOptions{})
}
