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
	"sort"
	"strings"
	"time"

	falco "kubeops.dev/falco-ui-server/apis/falco"
	falcov1alpha1 "kubeops.dev/falco-ui-server/apis/falco/v1alpha1"
	"kubeops.dev/ui-server/pkg/shared"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/request"
	kmapi "kmodules.xyz/client-go/api/v1"
	clustermeta "kmodules.xyz/client-go/cluster"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	sharedapi "kmodules.xyz/resource-metadata/apis/shared"
	"kmodules.xyz/resource-metadata/pkg/layouts"
	"kmodules.xyz/resource-metadata/pkg/tableconvertor"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func RenderLayout(
	ctx context.Context,
	kc client.Client,
	src kmapi.ObjectInfo,
	layoutName string, // optional
	pageName string, // optional
	convertToTable bool,
	renderBlocks sets.Set[string],
) (*rsapi.ResourceView, error) {
	srcRID, err := kmapi.ExtractResourceID(kc.RESTMapper(), src.Resource)
	if err != nil {
		return nil, err
	}
	var srcObj unstructured.Unstructured
	srcObj.SetGroupVersionKind(srcRID.GroupVersionKind())
	err = kc.Get(ctx, src.Ref.ObjectKey(), &srcObj)
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
		if bv, err := renderPageBlock(ctx, kc, srcRID, &srcObj, layout.Spec.Header, convertToTable); err != nil {
			return nil, err
		} else {
			out.Header = bv
		}
	}
	if layout.Spec.TabBar != nil && okToRender(layout.Spec.TabBar.Kind, renderBlocks) {
		if bv, err := renderPageBlock(ctx, kc, srcRID, &srcObj, layout.Spec.TabBar, convertToTable); err != nil {
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
			Name:     pageLayout.Name,
			Sections: make([]rsapi.ResourceSectionView, 0, len(pageLayout.Sections)),
		}
		for _, sectionLayout := range pageLayout.Sections {
			section := rsapi.ResourceSectionView{
				Name:    sectionLayout.Name,
				Info:    nil,
				Insight: nil,
				Blocks:  nil,
			}
			if sectionLayout.Info != nil && okToRender(sectionLayout.Info.Kind, renderBlocks) {
				if bv, err := renderPageBlock(ctx, kc, srcRID, &srcObj, sectionLayout.Info, convertToTable); err != nil {
					return nil, err
				} else {
					section.Info = bv
				}
			}
			if sectionLayout.Insight != nil && okToRender(sectionLayout.Insight.Kind, renderBlocks) {
				if bv, err := renderPageBlock(ctx, kc, srcRID, &srcObj, sectionLayout.Insight, convertToTable); err != nil {
					return nil, err
				} else {
					section.Insight = bv
				}
			}

			blocks := make([]rsapi.PageBlockView, 0, len(sectionLayout.Blocks))
			for _, block := range sectionLayout.Blocks {
				if okToRender(block.Kind, renderBlocks) {
					if bv, err := renderPageBlock(ctx, kc, srcRID, &srcObj, &block, convertToTable); err != nil {
						return nil, err
					} else {
						blocks = append(blocks, *bv)
					}
				}
			}
			section.Blocks = blocks

			page.Sections = append(page.Sections, section)
		}

		out.Pages = append(out.Pages, page)
	}

	return &out, nil
}

func okToRender(kind rsapi.TableKind, renderBlocks sets.Set[string]) bool {
	return renderBlocks.Len() == 0 || renderBlocks.Has(string(kind))
}

func RenderPageBlock(ctx context.Context, kc client.Client, src kmapi.ObjectInfo, block *rsapi.PageBlockLayout, convertToTable bool) (*rsapi.PageBlockView, error) {
	srcRID, err := kmapi.ExtractResourceID(kc.RESTMapper(), src.Resource)
	if err != nil {
		return nil, errors.Wrap(err, "failed to detect src resource id")
	}
	var srcObj unstructured.Unstructured
	srcObj.SetGroupVersionKind(srcRID.GroupVersionKind())
	err = kc.Get(ctx, src.Ref.ObjectKey(), &srcObj)
	if err != nil {
		return nil, err
	}

	return renderPageBlock(ctx, kc, srcRID, &srcObj, block, convertToTable)
}

func renderPageBlock(ctx context.Context, kc client.Client, srcRID *kmapi.ResourceID, srcObj *unstructured.Unstructured, block *rsapi.PageBlockLayout, convertToTable bool) (*rsapi.PageBlockView, error) {
	bv, err := _renderPageBlock(ctx, kc, srcRID, srcObj, block, convertToTable)
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

func _renderPageBlock(ctx context.Context, kc client.Client, srcRID *kmapi.ResourceID, srcObj *unstructured.Unstructured, block *rsapi.PageBlockLayout, convertToTable bool) (*rsapi.PageBlockView, error) {
	var impersonate bool
	if block != nil && block.ResourceLocator != nil && block.Impersonate {
		impersonate = true
	}
	cc, err := NewClient(ctx, kc, impersonate)
	if err != nil {
		return nil, err
	}

	out := rsapi.PageBlockView{
		Kind:    block.Kind,
		Name:    block.Name,
		Actions: block.Actions,
	}
	srcGVR := srcRID.GroupVersionResource()

	if block.Kind == rsapi.TableKindSelf || block.Kind == rsapi.TableKindSubTable {
		out.Resource = srcRID
		if convertToTable {
			converter, err := tableconvertor.New(block.FieldPath, block.View.Columns, renderDashboard(ctx, cc, srcObj), RenderExec(nil, &srcGVR))
			if err != nil {
				return &out, err
			}
			table, err := converter.ConvertToTable(ctx, srcObj)
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

	mapping, err := cc.RESTMapper().RESTMapping(schema.GroupKind{
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

	switch block.Query.Type {
	case sharedapi.GraphQLQuery:
		var objs []unstructured.Unstructured

		// handle FalcoEvent list call
		if vars[sharedapi.GraphQueryVarTargetGroup] == falco.GroupName &&
			vars[sharedapi.GraphQueryVarTargetKind] == falcov1alpha1.ResourceKindFalcoEvent {
			objs, err = listFalcoEvents(ctx, cc, block, srcID)
			if err != nil {
				return &out, err
			}
		} else {
			objs, err = ExecGraphQLQuery(cc, q, vars)
			if err != nil {
				return &out, err
			}
		}

		if convertToTable {
			converter, err := tableconvertor.New(block.FieldPath, block.View.Columns, renderDashboard(ctx, cc, srcObj), RenderExec(&srcGVR, &mapping.Resource))
			if err != nil {
				return &out, err
			}
			list := &unstructured.UnstructuredList{Items: objs}
			table, err := converter.ConvertToTable(ctx, list)
			if err != nil {
				return &out, err
			}
			if block.View.Sort != nil {
				idx := FindIndexFromColumnArray(table.Columns, block.View.Sort.FieldName)
				if idx != -1 {
					table.Rows = SortByIndex(table, block.View.Sort.Order, idx)
				}
			}
			out.Table = table
		} else {
			out.Items = objs
		}
	case sharedapi.RESTQuery:
		var obj map[string]any
		if q != "" {
			err = yaml.Unmarshal([]byte(q), &obj)
			if err != nil {
				return &out, errors.Wrapf(err, "failed to unmarshal query %s", q)
			}
		}
		u := unstructured.Unstructured{Object: obj}
		u.SetGroupVersionKind(mapping.GroupVersionKind)
		err = cc.Create(ctx, &u)
		if err != nil {
			return &out, err
		}

		if convertToTable {
			converter, err := tableconvertor.New(block.FieldPath, block.View.Columns, renderDashboard(ctx, cc, srcObj), RenderExec(&srcGVR, &mapping.Resource))
			if err != nil {
				return &out, err
			}
			table, err := converter.ConvertToTable(ctx, &u)
			if err != nil {
				return &out, err
			}
			out.Table = table
		} else {
			out.Items = []unstructured.Unstructured{u}
		}
	}

	user, found := request.UserFrom(ctx)
	if found {
		clientOrgResult, err := clustermeta.IsClientOrgMember(kc, user)
		if err != nil {
			return nil, err
		}
		if clientOrgResult.IsClientOrg {
			for _, col := range out.Table.Columns {
				if col.Dashboard != nil {
					col.Dashboard.Title = clustermeta.ClientDashboardTitle(col.Dashboard.Title)
				}
			}
		}
	}
	return &out, nil
}

func listFalcoEvents(ctx context.Context, kc client.Client, block *rsapi.PageBlockLayout, srcID *kmapi.ObjectID) ([]unstructured.Unstructured, error) {
	var refs []kmapi.ObjectReference
	var err error
	if srcID.Kind == "Pod" {
		refs = []kmapi.ObjectReference{
			{
				Name:      srcID.Name,
				Namespace: srcID.Namespace,
			},
		}
	} else {
		// list connected pods with this src
		refs, err = listPods(block, srcID)
		if err != nil {
			return nil, err
		}
	}

	var events []unstructured.Unstructured
	for _, pod := range refs {
		selector := labels.SelectorFromSet(map[string]string{
			"k8s.pod.name": pod.Name,
			"k8s.ns.name":  pod.Namespace,
		})

		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(falcov1alpha1.SchemeGroupVersion.WithKind(falcov1alpha1.ResourceKindFalcoEvent))
		err = kc.List(ctx, &list, &client.ListOptions{LabelSelector: selector})
		if meta.IsNoMatchError(err) {
			return nil, err
		} else if err == nil {
			events = append(events, list.Items...)
		}
	}
	return events, nil
}

func listPods(block *rsapi.PageBlockLayout, srcID *kmapi.ObjectID) ([]kmapi.ObjectReference, error) {
	block.Query.ByLabel = kmapi.EdgeLabelOffshoot
	podQ, podVars, err := block.GraphQuery(srcID.OID())
	if err != nil {
		return nil, err
	}

	podVars[sharedapi.GraphQueryVarTargetGroup] = ""
	podVars[sharedapi.GraphQueryVarTargetKind] = "Pod"
	pods, err := execRawGraphQLQuery(podQ, podVars)
	if err != nil {
		return nil, err
	}

	return pods, nil
}

func FindIndexFromColumnArray(cols []rsapi.ResourceColumn, fieldName string) int {
	for i, col := range cols {
		if col.Name == fieldName {
			return i
		}
	}
	return -1
}

func SortByIndex(table *rsapi.Table, order rsapi.TableSortOrder, idx int) []rsapi.TableRow {
	rows := table.Rows
	columnType := table.Columns[idx].Type

	if len(rows) == 0 || idx < 0 || idx >= len(rows[0].Cells) {
		return rows
	}
	sortedRows := make([]rsapi.TableRow, len(rows))
	copy(sortedRows, rows)

	parseDuration := func(data any) time.Duration {
		str, ok := data.(string)
		if !ok {
			return 0
		}
		str = strings.TrimSpace(strings.ToLower(str))
		multipliers := map[string]time.Duration{
			"s": time.Second,
			"m": time.Minute,
			"h": time.Hour,
			"d": time.Hour * 24,
			"y": time.Hour * 24 * 365,
		}
		for unit, multiplier := range multipliers {
			if strings.HasSuffix(str, unit) {
				numStr := strings.TrimSuffix(str, unit)
				var num int64
				_, err := fmt.Sscanf(numStr, "%d", &num)
				if err != nil {
					return 0
				}
				return time.Duration(num) * multiplier
			}
		}
		return 0
	}

	toFloat64 := func(v any) (float64, bool) {
		switch num := v.(type) {
		case int:
			return float64(num), true
		case int32:
			return float64(num), true
		case int64:
			return float64(num), true
		case float32:
			return float64(num), true
		case float64:
			return num, true
		default:
			return 0, false
		}
	}

	compare := func(a, b any) bool {
		if a == nil && b == nil {
			return false
		}
		if a == nil {
			return true
		}
		if b == nil {
			return false
		}

		switch columnType {
		case "date", "dateTime":
			durA := parseDuration(a)
			durB := parseDuration(b)
			if order == rsapi.TableSortOrderAscending {
				return durA < durB
			}
			return durA > durB
		case "string":
			c := strings.Compare(a.(string), b.(string))
			if order == rsapi.TableSortOrderAscending {
				return c < 0
			} else {
				return c > 0
			}
		// https://github.com/OAI/OpenAPI-Specification/blob/main/versions/2.0.md#data-types
		case "integer", "long", "float", "double":
			aVal, aOk := toFloat64(a)
			bVal, bOk := toFloat64(b)
			if !aOk {
				return true
			}
			if !bOk {
				return false
			}
			if order == rsapi.TableSortOrderAscending {
				return aVal < bVal
			}
			return aVal > bVal
		}
		return fmt.Sprintf("%v", a) > fmt.Sprintf("%v", b)
	}

	sort.Slice(sortedRows, func(i, j int) bool {
		dataI := sortedRows[i].Cells[idx].Data
		dataJ := sortedRows[j].Cells[idx].Data
		return compare(dataI, dataJ)
	})

	return sortedRows
}
