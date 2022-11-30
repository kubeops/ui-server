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
	openvizcs "go.openviz.dev/apimachinery/client/clientset/versioned"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	sharedapi "kmodules.xyz/resource-metadata/apis/shared"
	"kmodules.xyz/resource-metadata/pkg/layouts"
	"kmodules.xyz/resource-metadata/pkg/tableconvertor"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func RenderLayout(
	kc client.Client,
	oc openvizcs.Interface,
	src kmapi.ObjectInfo,
	layoutName string, // optional
	pageName string, // optional
	convertToTable bool,
	renderBlocks sets.String,
) (*rsapi.ResourceView, error) {
	srcRID, err := kmapi.ExtractResourceID(kc.RESTMapper(), src.Resource)
	if err != nil {
		return nil, err
	}
	var srcObj unstructured.Unstructured
	srcObj.SetGroupVersionKind(srcRID.GroupVersionKind())
	err = kc.Get(context.TODO(), src.Ref.ObjectKey(), &srcObj)
	if err != nil {
		return nil, err
	}

	var layout *rsapi.ResourceLayout
	if layoutName != "" {
		layout, err = layouts.LoadResourceLayout(kc, layoutName)
		if err != nil {
			return nil, err
		}
	} else {
		layout, err = layouts.LoadResourceLayoutForGVK(kc, srcRID.GroupVersionKind())
		if err != nil {
			return nil, err
		}
	}

	var out rsapi.ResourceView
	out.Resource = layout.Spec.Resource
	out.LayoutName = layout.Name
	if layout.Spec.UI != nil {
		out.UI = &sharedapi.UIParameters{
			Options:            layout.Spec.UI.Options,
			Editor:             layout.Spec.UI.Editor,
			InstanceLabelPaths: layout.Spec.UI.InstanceLabelPaths,
		}
		out.UI.Actions = make([]*sharedapi.ActionGroup, 0, len(layout.Spec.UI.Actions))
		for _, g := range layout.Spec.UI.Actions {
			g2 := sharedapi.ActionGroup{
				ActionInfo: g.ActionInfo,
				Items:      make([]sharedapi.Action, 0, len(g.Items)),
			}
			for _, a := range g.Items {
				a2 := sharedapi.Action{
					ActionInfo:  a.ActionInfo,
					Icons:       a.Icons,
					OperationID: a.OperationID,
					Flow:        a.Flow,
					Editor:      a.Editor,
				}
				tpl := strings.TrimSpace(a.DisabledTemplate)
				if tpl != "" {
					buf := shared.BufferPool.Get().(*bytes.Buffer)
					defer shared.BufferPool.Put(buf)

					result, err := shared.RenderTemplate(tpl, srcObj.UnstructuredContent(), buf)
					if err != nil {
						return nil, errors.Wrapf(err, "failed to test disabledTemplate for action %s", a.Name)
					}
					result = strings.TrimSpace(result)
					a2.Disabled = strings.EqualFold(result, "true")
				}

				g2.Items = append(g2.Items, a2)
			}
			out.UI.Actions = append(out.UI.Actions, &g2)
		}
	}

	if layout.Spec.Header != nil && okToRender(layout.Spec.Header.Kind, renderBlocks) {
		if bv, err := renderPageBlock(kc, oc, srcRID, &srcObj, layout.Spec.Header, convertToTable); err != nil {
			return nil, err
		} else {
			out.Header = bv
		}
	}
	if layout.Spec.TabBar != nil && okToRender(layout.Spec.TabBar.Kind, renderBlocks) {
		if bv, err := renderPageBlock(kc, oc, srcRID, &srcObj, layout.Spec.TabBar, convertToTable); err != nil {
			return nil, err
		} else {
			out.TabBar = bv
		}
	}

	out.Pages = make([]rsapi.ResourcePageView, 0, len(layout.Spec.Pages))

	for _, pageLayout := range layout.Spec.Pages {
		if pageName != "" && pageLayout.Name != pageName {
			continue
		}

		page := rsapi.ResourcePageView{
			Name:    pageLayout.Name,
			Info:    nil,
			Insight: nil,
			Blocks:  nil,
		}
		if pageLayout.Info != nil && okToRender(pageLayout.Info.Kind, renderBlocks) {
			if bv, err := renderPageBlock(kc, oc, srcRID, &srcObj, pageLayout.Info, convertToTable); err != nil {
				return nil, err
			} else {
				page.Info = bv
			}
		}
		if pageLayout.Insight != nil && okToRender(pageLayout.Insight.Kind, renderBlocks) {
			if bv, err := renderPageBlock(kc, oc, srcRID, &srcObj, pageLayout.Insight, convertToTable); err != nil {
				return nil, err
			} else {
				page.Insight = bv
			}
		}

		blocks := make([]rsapi.PageBlockView, 0, len(pageLayout.Blocks))
		for _, block := range pageLayout.Blocks {
			if okToRender(block.Kind, renderBlocks) {
				if bv, err := renderPageBlock(kc, oc, srcRID, &srcObj, &block, convertToTable); err != nil {
					return nil, err
				} else {
					blocks = append(blocks, *bv)
				}
			}
		}
		page.Blocks = blocks

		out.Pages = append(out.Pages, page)
	}

	return &out, nil
}

func okToRender(kind rsapi.TableKind, renderBlocks sets.String) bool {
	return renderBlocks.Len() == 0 || renderBlocks.Has(string(kind))
}

func RenderPageBlock(kc client.Client, oc openvizcs.Interface, src kmapi.ObjectInfo, block *rsapi.PageBlockLayout, convertToTable bool) (*rsapi.PageBlockView, error) {
	srcRID, err := kmapi.ExtractResourceID(kc.RESTMapper(), src.Resource)
	if err != nil {
		return nil, errors.Wrap(err, "failed to detect src resource id")
	}
	var srcObj unstructured.Unstructured
	srcObj.SetGroupVersionKind(srcRID.GroupVersionKind())
	err = kc.Get(context.TODO(), src.Ref.ObjectKey(), &srcObj)
	if err != nil {
		return nil, err
	}

	return renderPageBlock(kc, oc, srcRID, &srcObj, block, convertToTable)
}

func renderPageBlock(kc client.Client, oc openvizcs.Interface, srcRID *kmapi.ResourceID, srcObj *unstructured.Unstructured, block *rsapi.PageBlockLayout, convertToTable bool) (*rsapi.PageBlockView, error) {
	bv, err := _renderPageBlock(kc, oc, srcRID, srcObj, block, convertToTable)
	if err != nil {
		bv.Result = rsapi.RenderResult{
			Status:  rsapi.RenderError,
			Message: err.Error(),
		}
	} else if bv.Result.Status != rsapi.RenderMissing {
		bv.Result = rsapi.RenderResult{
			Status: rsapi.RenderSuccess,
		}
	}
	return bv, nil
}

func _renderPageBlock(kc client.Client, oc openvizcs.Interface, srcRID *kmapi.ResourceID, srcObj *unstructured.Unstructured, block *rsapi.PageBlockLayout, convertToTable bool) (*rsapi.PageBlockView, error) {
	out := rsapi.PageBlockView{
		Kind:    block.Kind,
		Name:    block.Name,
		Actions: block.Actions,
	}
	srcGVR := srcRID.GroupVersionResource()

	if block.Kind == rsapi.TableKindSelf || block.Kind == rsapi.TableKindSubTable {
		out.Resource = srcRID
		if convertToTable {
			converter, err := tableconvertor.New(block.FieldPath, block.View.Columns, renderDashboard(kc, oc, srcObj), RenderExec(nil, &srcGVR))
			if err != nil {
				return &out, err
			}
			table, err := converter.ConvertToTable(context.TODO(), srcObj)
			if err != nil {
				return &out, err
			}
			out.Table = table
		} else {
			out.Items = []unstructured.Unstructured{*srcObj}
		}
		return &out, nil
	} else if block.Kind != rsapi.TableKindConnection {
		return &out, fmt.Errorf("unsupported table kind found in block %+v", block)
	}

	mapping, err := kc.RESTMapper().RESTMapping(schema.GroupKind{
		Group: block.Ref.Group,
		Kind:  block.Ref.Kind,
	})
	if meta.IsNoMatchError(err) {
		out.Resource = &kmapi.ResourceID{
			Group: block.Ref.Group,
			// Version: "",
			// Name:    "",
			Kind: block.Ref.Kind,
			// Scope:   "",
		}
		out.Result = rsapi.RenderResult{
			Status: rsapi.RenderMissing,
		}
		if convertToTable {
			table := &rsapi.Table{
				Columns: make([]rsapi.ResourceColumn, 0, len(block.View.Columns)),
			}
			for _, def := range block.View.Columns {
				table.Columns = append(table.Columns, rsapi.Convert_ResourceColumnDefinition_To_ResourceColumn(def))
			}
			table.Rows = make([]rsapi.TableRow, 0)
			out.Table = table
		}
		return &out, nil
	} else if err != nil {
		return &out, err
	}

	out.Resource = kmapi.NewResourceID(mapping)

	srcID := kmapi.NewObjectID(srcObj)
	q, vars, err := block.GraphQuery(srcID.OID())
	if err != nil {
		return &out, err
	}

	if block.Query.Type == sharedapi.GraphQLQuery {
		objs, err := ExecGraphQLQuery(kc, q, vars)
		if err != nil {
			return &out, err
		}

		if convertToTable {
			converter, err := tableconvertor.New(block.FieldPath, block.View.Columns, renderDashboard(kc, oc, srcObj), RenderExec(&srcGVR, &mapping.Resource))
			if err != nil {
				return &out, err
			}
			list := &unstructured.UnstructuredList{Items: objs}
			table, err := converter.ConvertToTable(context.TODO(), list)
			if err != nil {
				return &out, err
			}
			out.Table = table
		} else {
			out.Items = objs
		}
	} else if block.Query.Type == sharedapi.RESTQuery {
		var obj map[string]interface{}
		if q != "" {
			err = yaml.Unmarshal([]byte(q), &obj)
			if err != nil {
				return &out, errors.Wrapf(err, "failed to unmarshal query %s", q)
			}
		}
		u := unstructured.Unstructured{Object: obj}
		u.SetGroupVersionKind(mapping.GroupVersionKind)
		err = kc.Create(context.TODO(), &u)
		if err != nil {
			return &out, err
		}

		if convertToTable {
			converter, err := tableconvertor.New(block.FieldPath, block.View.Columns, renderDashboard(kc, oc, srcObj), RenderExec(&srcGVR, &mapping.Resource))
			if err != nil {
				return &out, err
			}
			table, err := converter.ConvertToTable(context.TODO(), &u)
			if err != nil {
				return &out, err
			}
			out.Table = table
		} else {
			out.Items = []unstructured.Unstructured{u}
		}
	}
	return &out, nil
}
