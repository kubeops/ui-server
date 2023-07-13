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

package apiserver

import (
	"context"
	"errors"
	authzv1 "k8s.io/api/authorization/v1"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type APIAuthorizer struct {
	c client.Client
}

var _ authorizer.Authorizer = &APIAuthorizer{}

func New(c client.Client) *APIAuthorizer {
	return &APIAuthorizer{c: c}
}

func (a APIAuthorizer) Authorize(ctx context.Context, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	var sar authzv1.SubjectAccessReview

	if p := attrs.GetPath(); p != "" {
		sar.Spec.NonResourceAttributes = &authzv1.NonResourceAttributes{
			Path: p,
			Verb: attrs.GetVerb(),
		}
	} else {
		sar.Spec.ResourceAttributes = &authzv1.ResourceAttributes{
			Namespace:   attrs.GetNamespace(),
			Verb:        attrs.GetVerb(),
			Group:       attrs.GetAPIGroup(),
			Version:     attrs.GetAPIVersion(),
			Resource:    attrs.GetResource(),
			Subresource: attrs.GetSubresource(),
			Name:        attrs.GetName(),
		}
	}

	u := attrs.GetUser()
	sar.Spec.User = u.GetName()
	sar.Spec.Groups = u.GetGroups()
	sar.Spec.UID = u.GetUID()
	sar.Spec.Extra = make(map[string]authzv1.ExtraValue, len(u.GetExtra()))
	for k, v := range u.GetExtra() {
		sar.Spec.Extra[k] = v
	}

	err := a.c.Create(ctx, &sar)
	if err != nil {
		return authorizer.DecisionNoOpinion, "", err
	}

	if sar.Status.Allowed {
		return authorizer.DecisionAllow, sar.Status.Reason, nil
	}
	if sar.Status.Denied {
		return authorizer.DecisionDeny, sar.Status.Reason, nil
	}
	return authorizer.DecisionNoOpinion, sar.Status.Reason, errors.New(sar.Status.EvaluationError)
}
