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

package editormodel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	restclient "k8s.io/client-go/rest"
	editorapi "kmodules.xyz/resource-metadata/apis/editor/v1alpha1"
	"kubepack.dev/lib-app/pkg/editor"
	"sigs.k8s.io/controller-runtime/pkg/client"
	driversapi "x-helm.dev/apimachinery/apis/drivers/v1alpha1"
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
	return &Storage{cfg: cfg, scheme: scheme, mapper: mapper}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return editorapi.SchemeGroupVersion.WithKind(editorapi.ResourceKindEditorModel)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(editorapi.ResourceKindEditorModel)
}

func (r *Storage) New() runtime.Object {
	return &editorapi.EditorModel{}
}

func (r *Storage) Destroy() {}

// callerClient returns a controller-runtime client that impersonates the API
// caller, so the in-cluster reads done while reconstructing the editor model are
// authorized against the caller's own RBAC.
func (r *Storage) callerClient(ctx context.Context) (client.Client, error) {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info in request context")
	}
	cfg := restclient.CopyConfig(r.cfg)
	cfg.Impersonate = restclient.ImpersonationConfig{
		UserName: user.GetName(),
		UID:      user.GetUID(),
		Groups:   user.GetGroups(),
		Extra:    user.GetExtra(),
	}
	return client.New(cfg, client.Options{Scheme: r.scheme, Mapper: r.mapper})
}

// Create reconstructs the editor model for an existing installation from the
// chart values supplied in the request. The caller (b3) is responsible for the
// slow parts -- pulling the chart (getChart) and creating the AppRelease if
// missing -- so this method only performs fast in-cluster reads and stays within
// the aggregated apiserver request budget. Those reads run as the caller via
// impersonation, so they are authorized against the caller's own RBAC.
func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in, ok := obj.(*editorapi.EditorModel)
	if !ok {
		return nil, fmt.Errorf("invalid object type: %T", obj)
	}
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing request")
	}
	md := in.Request.Metadata

	values := map[string]any{}
	if in.Request.Values != nil && len(in.Request.Values.Raw) > 0 {
		if err := json.Unmarshal(in.Request.Values.Raw, &values); err != nil {
			return nil, apierrors.NewBadRequest(err.Error())
		}
	}

	kc, err := r.callerClient(ctx)
	if err != nil {
		return nil, err
	}

	var app driversapi.AppRelease
	if err := kc.Get(ctx, client.ObjectKey{Namespace: md.Release.Namespace, Name: md.Release.Name}, &app); err != nil {
		return nil, err
	}

	tpl, err := editor.EditorChartValueManifest(kc, &app, md, values)
	if err != nil {
		return nil, err
	}

	resp := &editorapi.EditorModelResponse{
		Manifest: string(tpl.Manifest),
	}
	if tpl.Values != nil {
		raw, err := json.Marshal(tpl.Values)
		if err != nil {
			return nil, err
		}
		resp.Values = &runtime.RawExtension{Raw: raw}
	}
	for _, res := range tpl.Resources {
		item := editorapi.EditorModelResource{
			Filename: res.Filename,
			Key:      res.Key,
		}
		if res.Data != nil {
			raw, err := json.Marshal(res.Data)
			if err != nil {
				return nil, err
			}
			item.Data = &runtime.RawExtension{Raw: raw}
		}
		resp.Resources = append(resp.Resources, item)
	}
	in.Response = resp

	return in, nil
}
