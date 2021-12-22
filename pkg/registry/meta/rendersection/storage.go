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

package rendersection

import (
	"context"

	"kubeops.dev/ui-server/pkg/graph"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/registry/rest"
	restclient "k8s.io/client-go/rest"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	cfg *restclient.Config
	kc  client.Client
	a   authorizer.Authorizer
}

var _ rest.GroupVersionKindProvider = &Storage{}
var _ rest.Scoper = &Storage{}
var _ rest.Creater = &Storage{}

func NewStorage(cfg *restclient.Config, kc client.Client, a authorizer.Authorizer) *Storage {
	return &Storage{
		cfg: cfg,
		kc:  kc,
		a:   a,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.ResourceKindRenderSection)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &v1alpha1.RenderSection{}
}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*v1alpha1.RenderSection)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}

	src := in.Request.Source.OID()
	section, err := graph.RenderSection(r.cfg, r.kc, src, in.Request.Target, in.Request.ConvertToTable)
	if err != nil {
		return nil, err
	}
	in.Response = &v1alpha1.RenderSectionResponse{
		PageSection: *section,
	}
	return in, nil
}
