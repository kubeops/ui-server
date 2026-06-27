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
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeAuthorizer struct {
	decision authorizer.Decision
	got      authorizer.Attributes
}

func (f *fakeAuthorizer) Authorize(_ context.Context, a authorizer.Attributes) (authorizer.Decision, string, error) {
	f.got = a
	return f.decision, "because test", nil
}

func ctxWithUser() context.Context {
	return apirequest.WithUser(context.Background(), &user.DefaultInfo{Name: "tester"})
}

func testMapper() meta.RESTMapper {
	m := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Group: "apps", Version: "v1"}})
	m.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	return m
}

func deploymentRequest() *rsapi.ResourceGraph {
	return &rsapi.ResourceGraph{
		Request: &rsapi.ResourceGraphRequest{
			Source: kmapi.ObjectInfo{
				Resource: kmapi.ResourceID{Group: "apps", Kind: "Deployment"},
				Ref:      kmapi.ObjectReference{Namespace: "ns1", Name: "dep1"},
			},
		},
	}
}

// TestCreateDeniedForUnreadableSource verifies the object graph is only returned to a
// caller allowed to "get" the source object.
func TestCreateDeniedForUnreadableSource(t *testing.T) {
	fa := &fakeAuthorizer{decision: authorizer.DecisionDeny}
	kc := fake.NewClientBuilder().WithRESTMapper(testMapper()).Build()
	r := NewStorage(kc, fa)

	_, err := r.Create(ctxWithUser(), deploymentRequest(), nil, nil)
	if !apierrors.IsForbidden(err) {
		t.Fatalf("expected Forbidden, got %v", err)
	}
	if fa.got.GetVerb() != "get" || fa.got.GetAPIGroup() != "apps" || fa.got.GetResource() != "deployments" ||
		fa.got.GetNamespace() != "ns1" || fa.got.GetName() != "dep1" {
		t.Errorf("unexpected authz attributes: verb=%q group=%q resource=%q ns=%q name=%q",
			fa.got.GetVerb(), fa.got.GetAPIGroup(), fa.got.GetResource(), fa.got.GetNamespace(), fa.got.GetName())
	}
}

func TestCreateMissingUserIsBadRequest(t *testing.T) {
	fa := &fakeAuthorizer{decision: authorizer.DecisionAllow}
	kc := fake.NewClientBuilder().WithRESTMapper(testMapper()).Build()
	r := NewStorage(kc, fa)

	_, err := r.Create(context.Background(), deploymentRequest(), nil, nil)
	if !apierrors.IsBadRequest(err) {
		t.Fatalf("expected BadRequest for missing user, got %v", err)
	}
}
