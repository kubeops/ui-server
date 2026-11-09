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

package resourcegraph

import (
	"context"
	"fmt"
	"strings"

	"kubeops.dev/ui-server/pkg/graph"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/endpoints/request"
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
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindResourceGraph)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rsapi.ResourceKindResourceGraph)
}

func (r *Storage) New() runtime.Object {
	return &rsapi.ResourceGraph{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*rsapi.ResourceGraph)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}

	u, ok := request.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	rid, err := kmapi.ExtractResourceID(r.kc.RESTMapper(), in.Request.Source.Resource)
	if err != nil {
		return nil, err
	}
	src := kmapi.ObjectID{
		Group:     rid.Group,
		Kind:      rid.Kind,
		Namespace: in.Request.Source.Ref.Namespace,
		Name:      in.Request.Source.Ref.Name,
	}
	// The caller must be able to get the source object before we return its connections.
	srcGK := schema.GroupKind{Group: rid.Group, Kind: rid.Kind}
	if allowed, gr, err := graph.CanAccess(ctx, r.a, u, r.kc.RESTMapper(), "get", srcGK, src.Namespace, src.Name); err != nil {
		return nil, apierrors.NewInternalError(err)
	} else if !allowed {
		return nil, apierrors.NewForbidden(gr, src.Name, fmt.Errorf("user %q cannot get %s %s/%s", u.GetName(), rid.Kind, src.Namespace, src.Name))
	}

	resp, err := graph.ResourceGraph(r.kc.RESTMapper(), src, kmapi.EdgeLabelValues())
	if err != nil {
		return nil, err
	}
	// Drop connections to objects the caller cannot get, so the graph does not leak the
	// names of related objects outside their reach. Resources holds kinds only, so it stays.
	resp.Connections = r.filterConnections(ctx, u, resp)
	in.Response = resp
	return in, nil
}

func (r *Storage) filterConnections(ctx context.Context, u user.Info, resp *rsapi.ResourceGraphResponse) []rsapi.ObjectConnection {
	decided := map[string]bool{}
	canGet := func(ptr rsapi.ObjectPointer) bool {
		if ptr.ResourceID < 0 || ptr.ResourceID >= len(resp.Resources) {
			return false
		}
		res := resp.Resources[ptr.ResourceID]
		key := fmt.Sprintf("%s/%s/%s/%s", res.Group, res.Kind, ptr.Namespace, ptr.Name)
		if v, ok := decided[key]; ok {
			return v
		}
		allowed, _, err := graph.CanAccess(ctx, r.a, u, r.kc.RESTMapper(), "get", schema.GroupKind{Group: res.Group, Kind: res.Kind}, ptr.Namespace, ptr.Name)
		v := err == nil && allowed
		decided[key] = v
		return v
	}

	kept := resp.Connections[:0]
	for _, c := range resp.Connections {
		if canGet(c.Source) && canGet(c.Target) {
			kept = append(kept, c)
		}
	}
	return kept
}
