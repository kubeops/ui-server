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

package renderrawgraph

import (
	"context"
	"errors"
	"strings"

	"kubeops.dev/ui-server/pkg/graph"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
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
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindRenderRawGraph)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rsapi.ResourceKindRenderRawGraph)
}

func (r *Storage) New() runtime.Object {
	return &rsapi.RenderRawGraph{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*rsapi.RenderRawGraph)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}

	var oid kmapi.OID
	if in.Request.Source != nil {
		rid := in.Request.Source.Resource
		if rid.Kind == "" {
			r2, err := kmapi.ExtractResourceID(r.kc.RESTMapper(), in.Request.Source.Resource)
			if err != nil {
				return nil, err
			}
			rid = *r2
		}
		src := kmapi.ObjectID{
			Group:     rid.Group,
			Kind:      rid.Kind,
			Namespace: in.Request.Source.Ref.Namespace,
			Name:      in.Request.Source.Ref.Name,
		}

		// The graph response exposes the names and namespaces of related objects across
		// the cluster, so only serve it to callers who can read the source object.
		if err := r.authorizeGetSource(ctx, src); err != nil {
			return nil, err
		}
		oid = src.OID()
	} else if err := r.authorizeClusterRead(ctx); err != nil {
		// Without a source the whole cluster graph is rendered; require cluster-wide read access.
		return nil, err
	}

	resp, err := graph.Render(oid)
	if err != nil {
		return nil, err
	}
	in.Response = resp
	return in, nil
}

// authorizeGetSource ensures the requesting user is allowed to "get" the source object
// before its object graph is returned.
func (r *Storage) authorizeGetSource(ctx context.Context, src kmapi.ObjectID) error {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return apierrors.NewBadRequest("missing user info in request context")
	}

	mapping, err := r.kc.RESTMapper().RESTMapping(schema.GroupKind{Group: src.Group, Kind: src.Kind})
	if err != nil {
		return apierrors.NewInternalError(err)
	}

	attrs := authorizer.AttributesRecord{
		User:            user,
		Verb:            "get",
		APIGroup:        mapping.Resource.Group,
		Resource:        mapping.Resource.Resource,
		Namespace:       src.Namespace,
		Name:            src.Name,
		ResourceRequest: true,
	}
	decision, why, err := r.a.Authorize(ctx, attrs)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	if decision != authorizer.DecisionAllow {
		return apierrors.NewForbidden(mapping.Resource.GroupResource(), src.Name, errors.New(why))
	}
	return nil
}

// authorizeClusterRead requires cluster-wide read access, used when the whole cluster
// graph (no specific source) is requested. Only callers granted "get" on all resources
// in all groups (e.g. cluster-admin) are allowed.
func (r *Storage) authorizeClusterRead(ctx context.Context) error {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return apierrors.NewBadRequest("missing user info in request context")
	}

	attrs := authorizer.AttributesRecord{
		User:            user,
		Verb:            "get",
		APIGroup:        "*",
		Resource:        "*",
		ResourceRequest: true,
	}
	decision, why, err := r.a.Authorize(ctx, attrs)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	if decision != authorizer.DecisionAllow {
		return apierrors.NewForbidden(schema.GroupResource{Resource: rsapi.ResourceRenderRawGraphs}, "", errors.New(why))
	}
	return nil
}
