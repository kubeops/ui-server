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

package resourcequery

import (
	"context"
	"fmt"
	"strings"

	"kubeops.dev/ui-server/pkg/graph"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/pkg/tableconvertor"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc client.Client
	a  authorizer.Authorizer
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc: kc,
		a:  a,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindResourceQuery)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rsapi.ResourceKindResourceQuery)
}

func (r *Storage) New() runtime.Object {
	return &rsapi.ResourceQuery{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*rsapi.ResourceQuery)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}
	req := in.Request

	rid, err := kmapi.ExtractResourceID(r.kc.RESTMapper(), req.Source.Resource)
	if err != nil {
		return nil, err
	}

	gvk := rid.GroupVersionKind()
	if req.Source.Selector == nil {
		var out unstructured.Unstructured
		out.SetGroupVersionKind(gvk)
		err := r.kc.Get(context.TODO(), client.ObjectKey{Namespace: req.Source.Namespace, Name: req.Source.Name}, &out)
		if err != nil {
			return nil, err
		}
		if req.Target == nil {
			resp, err := r.toOutput(rid, &out, req.OutputFormat)
			if err != nil {
				return nil, err
			}
			in.Response = resp
		} else {
			src := kmapi.NewObjectID(&out)

			if req.OutputFormat == rsapi.OutputFormatRef {
				_, refs, err := graph.ExecRawQuery(r.kc, src.OID(), *req.Target)
				if err != nil {
					return nil, err
				}
				data, err := json.Marshal(refs)
				if err != nil {
					return nil, err
				}
				in.Response = &runtime.RawExtension{Raw: data}
			} else {
				rid2, items, err := graph.ExecQuery(r.kc, src.OID(), *req.Target)
				if err != nil {
					return nil, err
				}

				resp, err := r.toOutput(rid2, &unstructured.UnstructuredList{Items: items}, req.OutputFormat)
				if err != nil {
					return nil, err
				}
				in.Response = resp
			}
		}
	} else {
		selector, err := metav1.LabelSelectorAsSelector(req.Source.Selector)
		if err != nil {
			return nil, err
		}
		opts := client.ListOptions{
			Namespace:     req.Source.Namespace,
			LabelSelector: selector,
		}
		var out unstructured.UnstructuredList
		out.SetGroupVersionKind(gvk)
		err = r.kc.List(context.TODO(), &out, &opts)
		if err != nil {
			return nil, err
		}

		if req.Target == nil {
			resp, err := r.toOutput(rid, &out, req.OutputFormat)
			if err != nil {
				return nil, err
			}
			in.Response = resp
		} else {
			return nil, apierrors.NewBadRequest("request.selector can't be used for target queries")
		}
	}

	return in, nil
}

func (r *Storage) toOutput(rid *kmapi.ResourceID, src runtime.Object, f rsapi.OutputFormat) (*runtime.RawExtension, error) {
	gvr := rid.GroupVersionResource()
	switch f {
	case rsapi.OutputFormatTable:
		if meta.IsListType(src) {
			items, err := meta.GetItemsPtr(src)
			if err != nil {
				return nil, err
			}

			table, err := tableconvertor.TableForList(r.kc, rid.GroupVersionResource(), items.([]unstructured.Unstructured), "", nil, graph.RenderExec(nil, &gvr))
			if err != nil {
				return nil, err
			}
			return &runtime.RawExtension{Object: table}, nil
		}
		table, err := tableconvertor.TableForObject(r.kc, src, "", nil, graph.RenderExec(nil, &gvr))
		if err != nil {
			return nil, err
		}
		return &runtime.RawExtension{Object: table}, nil
	case rsapi.OutputFormatRef:
		if meta.IsListType(src) {
			refs := make([]kmapi.ObjectReference, 0, meta.LenList(src))

			err := meta.EachListItem(src, func(o runtime.Object) error {
				if obj, ok := o.(client.Object); ok {
					refs = append(refs, kmapi.ObjectReference{
						Namespace: obj.GetNamespace(),
						Name:      obj.GetName(),
					})
				}
				return nil
			})
			if err != nil {
				return nil, err
			}

			data, err := json.Marshal(refs)
			if err != nil {
				return nil, err
			}
			return &runtime.RawExtension{Raw: data}, nil
		}

		if s, ok := src.(client.Object); ok {
			ref := kmapi.ObjectReference{
				Namespace: s.GetNamespace(),
				Name:      s.GetName(),
			}
			data, err := json.Marshal(ref)
			if err != nil {
				return nil, err
			}
			return &runtime.RawExtension{Raw: data}, nil
		}
		return nil, fmt.Errorf("%+v is not a client.Object", src)
	case rsapi.OutputFormatObject, "":
		return &runtime.RawExtension{Object: src}, nil
	default:
		return nil, fmt.Errorf("unknown output format %s", f)
	}
}
