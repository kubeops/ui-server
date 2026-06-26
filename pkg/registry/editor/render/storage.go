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
	"encoding/json"
	"fmt"
	"strings"

	"kubeops.dev/ui-server/pkg/registry/editor/editorutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	restclient "k8s.io/client-go/rest"
	meta_util "kmodules.xyz/client-go/meta"
	editorapi "kmodules.xyz/resource-metadata/apis/editor/v1alpha1"
	"kubepack.dev/lib-app/pkg/editor"
	"kubepack.dev/lib-helm/pkg/repo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	releasesapi "x-helm.dev/apimachinery/apis/releases/v1alpha1"
)

type Storage struct {
	cfg    *restclient.Config
	scheme *runtime.Scheme
	mapper meta.RESTMapper
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(cfg *restclient.Config, scheme *runtime.Scheme, mapper meta.RESTMapper) *Storage {
	return &Storage{
		cfg:    cfg,
		scheme: scheme,
		mapper: mapper,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return editorapi.SchemeGroupVersion.WithKind(editorapi.ResourceKindEditorRender)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(editorapi.ResourceKindEditorRender)
}

func (r *Storage) New() runtime.Object {
	return &editorapi.EditorRender{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in, ok := obj.(*editorapi.EditorRender)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid object type: %T", obj))
	}
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing request")
	}
	req := in.Request

	opts, err := decodeOptions(req.Options)
	if err != nil {
		return nil, apierrors.NewBadRequest(err.Error())
	}

	kc, reg, err := editorutil.CallerClient(ctx, r.cfg, r.scheme, r.mapper)
	if err != nil {
		return nil, err
	}

	var resp editorapi.EditorRenderResponse
	switch req.Output {
	case editorapi.EditorOutputModel:
		model, err := editor.GenerateResourceEditorModel(kc, reg, opts)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(model)
		if err != nil {
			return nil, err
		}
		resp.Model = &runtime.RawExtension{Raw: raw}
	case editorapi.EditorOutputManifest:
		manifest, _, err := editor.RenderResourceEditorChart(kc, reg, opts)
		if err != nil {
			return nil, err
		}
		resp.Manifest = manifest
	case editorapi.EditorOutputResources, "":
		tpl, err := renderTemplate(kc, reg, req.ChartRef, opts)
		if err != nil {
			return nil, err
		}
		out, err := editorutil.RenderedResourceOutput(tpl.CRDs, tpl.Resources, req.SkipCRDs, meta_util.YAMLFormat)
		if err != nil {
			return nil, err
		}
		resp.Resources = out
	default:
		return nil, apierrors.NewBadRequest(fmt.Sprintf("unsupported output: %q", req.Output))
	}

	in.Response = &resp
	return in, nil
}

func renderTemplate(kc client.Client, reg repo.IRegistry, chartRef *releasesapi.ChartSourceFlatRef, opts map[string]any) (*releasesapi.ChartTemplate, error) {
	if chartRef != nil && chartRef.Name != "" {
		_, tpl, err := editor.RenderChart(kc, reg, chartRef.ToAPIObject(), opts)
		return tpl, err
	}
	_, tpl, err := editor.RenderResourceEditorChart(kc, reg, opts)
	return tpl, err
}

func decodeOptions(raw *runtime.RawExtension) (map[string]any, error) {
	opts := map[string]any{}
	if raw == nil || len(raw.Raw) == 0 {
		return opts, nil
	}
	if err := json.Unmarshal(raw.Raw, &opts); err != nil {
		return nil, err
	}
	return opts, nil
}
