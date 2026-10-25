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
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
)

type fakeAuthorizer struct {
	decision authorizer.Decision
	err      error
	got      authorizer.Attributes
}

func (f *fakeAuthorizer) Authorize(_ context.Context, a authorizer.Attributes) (authorizer.Decision, string, error) {
	f.got = a
	return f.decision, "because test", f.err
}

func ctxWithUser() context.Context {
	return apirequest.WithUser(context.Background(), &user.DefaultInfo{Name: "tester"})
}

func TestAuthorizeAllow(t *testing.T) {
	fa := &fakeAuthorizer{decision: authorizer.DecisionAllow}
	err := Authorize(ctxWithUser(), fa, authorizer.AttributesRecord{Verb: "get", Resource: "pods"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !fa.got.IsResourceRequest() {
		t.Error("expected ResourceRequest to be set")
	}
	if fa.got.GetUser() == nil || fa.got.GetUser().GetName() != "tester" {
		t.Errorf("user not propagated to authorizer: %+v", fa.got.GetUser())
	}
}

func TestAuthorizeDenyIsForbidden(t *testing.T) {
	fa := &fakeAuthorizer{decision: authorizer.DecisionDeny}
	err := Authorize(ctxWithUser(), fa, authorizer.AttributesRecord{Verb: "get", Resource: "pods"})
	if !apierrors.IsForbidden(err) {
		t.Fatalf("expected Forbidden, got %v", err)
	}
}

func TestAuthorizeMissingUserIsBadRequest(t *testing.T) {
	fa := &fakeAuthorizer{decision: authorizer.DecisionAllow}
	err := Authorize(context.Background(), fa, authorizer.AttributesRecord{Verb: "get", Resource: "pods"})
	if !apierrors.IsBadRequest(err) {
		t.Fatalf("expected BadRequest, got %v", err)
	}
}

func TestClusterReadAttributes(t *testing.T) {
	a := ClusterReadAttributes()
	if a.Verb != "get" || a.APIGroup != "*" || a.Resource != "*" {
		t.Errorf("unexpected cluster read attributes: %+v", a)
	}
}
