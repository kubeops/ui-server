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
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/pkg/layouts"
	"kmodules.xyz/resource-metadata/pkg/tableconvertor"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func RenderLayout(
	kc client.Client,
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
	out.UI = layout.Spec.UI

	if layout.Spec.Header != nil && okToRender(layout.Spec.Header.Kind, renderBlocks) {
		if bv, err := renderPageBlock(kc, srcRID, &srcObj, layout.Spec.Header, convertToTable); err != nil {
			return nil, err
		} else {
			out.Header = bv
		}
	}
	if layout.Spec.TabBar != nil && okToRender(layout.Spec.TabBar.Kind, renderBlocks) {
		if bv, err := renderPageBlock(kc, srcRID, &srcObj, layout.Spec.TabBar, convertToTable); err != nil {
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
			if bv, err := renderPageBlock(kc, srcRID, &srcObj, pageLayout.Info, convertToTable); err != nil {
				return nil, err
			} else {
				page.Info = bv
			}
		}
		if pageLayout.Insight != nil && okToRender(pageLayout.Insight.Kind, renderBlocks) {
			if bv, err := renderPageBlock(kc, srcRID, &srcObj, pageLayout.Insight, convertToTable); err != nil {
				return nil, err
			} else {
				page.Insight = bv
			}
		}

		blocks := make([]rsapi.PageBlockView, 0, len(pageLayout.Blocks))
		for _, block := range pageLayout.Blocks {
			if okToRender(block.Kind, renderBlocks) {
				if bv, err := renderPageBlock(kc, srcRID, &srcObj, &block, convertToTable); err != nil {
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

func RenderPageBlock(kc client.Client, src kmapi.ObjectInfo, block *rsapi.PageBlockLayout, convertToTable bool) (*rsapi.PageBlockView, error) {
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

	return renderPageBlock(kc, srcRID, &srcObj, block, convertToTable)
}

func renderPageBlock(kc client.Client, srcRID *kmapi.ResourceID, srcObj *unstructured.Unstructured, block *rsapi.PageBlockLayout, convertToTable bool) (*rsapi.PageBlockView, error) {
	bv, err := _renderPageBlock(kc, srcRID, srcObj, block, convertToTable)
	if err != nil {
		bv.Result = rsapi.RenderResult{
			Status:  rsapi.RenderError,
			Message: err.Error(),
		}
	} else {
		bv.Result = rsapi.RenderResult{
			Status: rsapi.RenderSuccess,
		}
	}
	return bv, nil
}

func _renderPageBlock(kc client.Client, srcRID *kmapi.ResourceID, srcObj *unstructured.Unstructured, block *rsapi.PageBlockLayout, convertToTable bool) (*rsapi.PageBlockView, error) {
	out := rsapi.PageBlockView{
		Kind:    block.Kind,
		Name:    block.Name,
		Actions: block.Actions,
	}

	if block.Kind == rsapi.TableKindSelf || block.Kind == rsapi.TableKindSubTable {
		out.Resource = srcRID
		if convertToTable {
			converter, err := tableconvertor.New(block.FieldPath, block.View.Columns)
			if err != nil {
				return nil, err
			}
			table, err := converter.ConvertToTable(context.TODO(), srcObj, nil)
			if err != nil {
				return nil, err
			}
			out.Table = table
		} else {
			out.Items = []unstructured.Unstructured{*srcObj}
		}
		return &out, nil
	} else if block.Kind != rsapi.TableKindConnection {
		return nil, fmt.Errorf("unsupported table kind found in block %+v", block)
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
		return nil, err
	}

	out.Resource = kmapi.NewResourceID(mapping)

	srcID := kmapi.NewObjectID(srcObj)
	q, vars, err := block.GraphQuery(srcID.OID())
	if err != nil {
		return nil, err
	}

	if block.Query.Type == rsapi.GraphQLQuery {
		objs, err := ExecGraphQLQuery(kc, q, vars)
		if err != nil {
			return nil, err
		}

		if convertToTable {
			converter, err := tableconvertor.New(block.FieldPath, block.View.Columns)
			if err != nil {
				return nil, err
			}
			list := &unstructured.UnstructuredList{Items: objs}
			table, err := converter.ConvertToTable(context.TODO(), list, nil)
			if err != nil {
				return nil, err
			}
			out.Table = table
		} else {
			out.Items = objs
		}
	} else if block.Query.Type == rsapi.RESTQuery {
		var obj unstructured.Unstructured
		if q != "" {
			err = yaml.Unmarshal([]byte(q), &obj)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to unmarshal query %s", q)
			}
		}
		obj.SetGroupVersionKind(mapping.GroupVersionKind)
		err = kc.Create(context.TODO(), &obj)
		if err != nil {
			return nil, err
		}

		if convertToTable {
			converter, err := tableconvertor.New(block.FieldPath, block.View.Columns)
			if err != nil {
				return nil, err
			}
			table, err := converter.ConvertToTable(context.TODO(), &obj, nil)
			if err != nil {
				return nil, err
			}
			out.Table = table
		} else {
			out.Items = []unstructured.Unstructured{obj}
		}
	}
	return &out, nil
}
