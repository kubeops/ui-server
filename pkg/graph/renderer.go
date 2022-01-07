package graph

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiv1 "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/pkg/layouts"
	"kmodules.xyz/resource-metadata/pkg/tableconvertor"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func RenderLayout(
	kc client.Client,
	src apiv1.ObjectInfo,
	layoutName string, // optional
	pageName string, // optional
	convertToTable bool,
	renderSelfOnly bool,
) (*v1alpha1.ResourceView, error) {

	srcRID, err := apiv1.ExtractResourceID(kc.RESTMapper(), src.Resource)
	if err != nil {
		return nil, err
	}
	var srcObj unstructured.Unstructured
	srcObj.SetGroupVersionKind(srcRID.GroupVersionKind())
	err = kc.Get(context.TODO(), src.Ref.ObjectKey(), &srcObj)
	if err != nil {
		return nil, err
	}

	var layout *v1alpha1.ResourceLayout
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

	var out v1alpha1.ResourceView
	out.Resource = layout.Spec.Resource
	out.LayoutName = layout.Name
	out.UI = layout.Spec.UI

	if layout.Spec.Header != nil && okToRender(layout.Spec.Header.Kind, renderSelfOnly) {
		if bv, err := renderPageBlock(kc, srcRID, &srcObj, layout.Spec.Header, convertToTable); err != nil {
			return nil, err
		} else {
			out.Header = bv
		}
	}
	if layout.Spec.TabBar != nil && okToRender(layout.Spec.TabBar.Kind, renderSelfOnly) {
		if bv, err := renderPageBlock(kc, srcRID, &srcObj, layout.Spec.TabBar, convertToTable); err != nil {
			return nil, err
		} else {
			out.TabBar = bv
		}
	}

	out.Pages = make([]v1alpha1.ResourcePageView, 0, len(layout.Spec.Pages))

	for _, pageLayout := range layout.Spec.Pages {
		if pageName != "" && pageLayout.Name != pageName {
			continue
		}

		page := v1alpha1.ResourcePageView{
			Name:    pageLayout.Name,
			Info:    nil,
			Insight: nil,
			Blocks:  nil,
		}
		if pageLayout.Info != nil && okToRender(pageLayout.Info.Kind, renderSelfOnly) {
			if bv, err := renderPageBlock(kc, srcRID, &srcObj, pageLayout.Info, convertToTable); err != nil {
				return nil, err
			} else {
				page.Info = bv
			}
		}
		if pageLayout.Insight != nil && okToRender(pageLayout.Insight.Kind, renderSelfOnly) {
			if bv, err := renderPageBlock(kc, srcRID, &srcObj, pageLayout.Insight, convertToTable); err != nil {
				return nil, err
			} else {
				page.Insight = bv
			}
		}

		blocks := make([]v1alpha1.PageBlockView, 0, len(pageLayout.Blocks))
		for _, block := range pageLayout.Blocks {
			if okToRender(block.Kind, renderSelfOnly) {
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

func okToRender(kind v1alpha1.TableKind, renderSelfOnly bool) bool {
	if renderSelfOnly {
		return kind == v1alpha1.TableKindSelf || kind == v1alpha1.TableKindSubTable
	}
	return true
}

func RenderPageBlock(kc client.Client, src apiv1.ObjectInfo, block *v1alpha1.PageBlockLayout, convertToTable bool) (*v1alpha1.PageBlockView, error) {
	srcRID, err := apiv1.ExtractResourceID(kc.RESTMapper(), src.Resource)
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

func renderPageBlock(kc client.Client, srcRID *apiv1.ResourceID, srcObj *unstructured.Unstructured, block *v1alpha1.PageBlockLayout, convertToTable bool) (*v1alpha1.PageBlockView, error) {
	srcID := apiv1.NewObjectID(srcObj)

	var out v1alpha1.PageBlockView
	if block.Kind == v1alpha1.TableKindSelf || block.Kind == v1alpha1.TableKindSubTable {
		out = v1alpha1.PageBlockView{
			Kind:     block.Kind,
			Name:     block.Name,
			Resource: srcRID,
			Actions:  block.Actions,
		}
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
	} else if block.Kind != v1alpha1.TableKindConnection {
		return nil, fmt.Errorf("unsupported table kind found in block %+v", block)
	}

	mapping, err := kc.RESTMapper().RESTMapping(schema.GroupKind{
		Group: block.Ref.Group,
		Kind:  block.Ref.Kind,
	})
	if err != nil {
		return nil, err
	}
	out.Resource = apiv1.NewResourceID(mapping)

	q, vars, err := block.GraphQuery(srcID.OID())
	if err != nil {
		return nil, err
	}

	if block.Query.Type == v1alpha1.GraphQLQuery {
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
	} else if block.Query.Type == v1alpha1.RESTQuery {
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
