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

package graph

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// authzClient reads through the wrapped (server-credentialed, cache-backed) client but
// authorizes every access against the caller with the apiserver authorizer, so a cached
// read never returns an object the caller could not get on their own. Get denies with
// Forbidden; List drops the items the caller cannot get; Create authorizes the create verb.
type authzClient struct {
	client.Client
	authz authorizer.Authorizer
	user  user.Info
}

// NewAuthorizedClient wraps kc so that reads are served from kc (the server SA, which may be
// cache-backed) but each object is authorized for u first.
func NewAuthorizedClient(kc client.Client, authz authorizer.Authorizer, u user.Info) client.Client {
	return &authzClient{Client: kc, authz: authz, user: u}
}

// IgnoreForbidden lets callers that should skip objects the caller cannot see (e.g. graph
// ref expansion) treat a denial like a not-found instead of failing the whole request.
func IgnoreForbidden(err error) error {
	if apierrors.IsForbidden(err) {
		return nil
	}
	return err
}

// CanAccess reports whether u may perform verb on the object identified by gk/namespace/name,
// resolving the resource via mapper and asking the apiserver authorizer. It also returns the
// resolved GroupResource for building a Forbidden error.
func CanAccess(ctx context.Context, authz authorizer.Authorizer, u user.Info, mapper meta.RESTMapper, verb string, gk schema.GroupKind, namespace, name string) (bool, schema.GroupResource, error) {
	mapping, err := mapper.RESTMapping(gk)
	if err != nil {
		return false, schema.GroupResource{Group: gk.Group, Resource: gk.Kind}, err
	}
	gr := mapping.Resource.GroupResource()
	attrs := authorizer.AttributesRecord{
		User:            u,
		Verb:            verb,
		APIGroup:        mapping.Resource.Group,
		APIVersion:      mapping.Resource.Version,
		Resource:        mapping.Resource.Resource,
		Namespace:       namespace,
		Name:            name,
		ResourceRequest: true,
	}
	decision, _, err := authz.Authorize(ctx, attrs)
	if err != nil {
		return false, gr, err
	}
	return decision == authorizer.DecisionAllow, gr, nil
}

func (c *authzClient) allowed(ctx context.Context, verb string, gvk schema.GroupVersionKind, namespace, name string) (bool, schema.GroupResource, error) {
	return CanAccess(ctx, c.authz, c.user, c.RESTMapper(), verb, gvk.GroupKind(), namespace, name)
}

func (c *authzClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}
	ok, gr, err := c.allowed(ctx, "get", gvk, key.Namespace, key.Name)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	if !ok {
		return apierrors.NewForbidden(gr, key.Name, fmt.Errorf("user %q cannot get %s %s/%s", c.user.GetName(), gvk.Kind, key.Namespace, key.Name))
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *authzClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if err := c.Client.List(ctx, list, opts...); err != nil {
		return err
	}
	gvk, err := c.GroupVersionKindFor(list)
	if err != nil {
		return err
	}
	itemGVK := gvk
	itemGVK.Kind = strings.TrimSuffix(gvk.Kind, "List")

	items, err := meta.ExtractList(list)
	if err != nil {
		return err
	}
	kept := items[:0]
	for _, item := range items {
		acc, err := meta.Accessor(item)
		if err != nil {
			return err
		}
		ok, _, err := c.allowed(ctx, "get", itemGVK, acc.GetNamespace(), acc.GetName())
		if err != nil {
			return apierrors.NewInternalError(err)
		}
		if ok {
			kept = append(kept, item)
		}
	}
	return meta.SetList(list, kept)
}

func (c *authzClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return err
	}
	ok, gr, err := c.allowed(ctx, "create", gvk, obj.GetNamespace(), obj.GetName())
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	if !ok {
		return apierrors.NewForbidden(gr, obj.GetName(), fmt.Errorf("user %q cannot create %s", c.user.GetName(), gvk.Kind))
	}
	return c.Client.Create(ctx, obj, opts...)
}
