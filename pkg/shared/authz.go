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

package shared

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
)

// Authorize resolves the requesting user from ctx, fills it into attrs, and asks the
// authorizer for a decision. It returns a Forbidden error when access is denied so the
// aggregated apiserver surfaces a proper 403 to the caller.
//
// Callers only need to set the resource-identifying fields of attrs (Verb, APIGroup,
// Resource, Namespace, Name); User and ResourceRequest are set here.
func Authorize(ctx context.Context, a authorizer.Authorizer, attrs authorizer.AttributesRecord) error {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return apierrors.NewBadRequest("missing user info in request context")
	}
	attrs.User = user
	attrs.ResourceRequest = true

	decision, why, err := a.Authorize(ctx, attrs)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	if decision != authorizer.DecisionAllow {
		gr := schema.GroupResource{Group: attrs.APIGroup, Resource: attrs.Resource}
		return apierrors.NewForbidden(gr, attrs.Name, errors.New(why))
	}
	return nil
}

// ClusterReadAttributes returns attributes representing cluster-wide read access
// (get on all resources in all groups). Only callers effectively granted cluster-admin
// satisfy this check. Used to gate endpoints that expose whole-cluster data with no
// single resource to authorize against.
func ClusterReadAttributes() authorizer.AttributesRecord {
	return authorizer.AttributesRecord{
		Verb:     "get",
		APIGroup: "*",
		Resource: "*",
	}
}
