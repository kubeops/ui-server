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

package reports

import (
	"context"
	"testing"

	costapi "kubeops.dev/ui-server/apis/cost/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
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

// TestCreateDeniedWithoutClusterReadAccess verifies the cost report requires cluster-wide
// read access: a denied authorizer yields a Forbidden response, and the check asks for
// "get" on */*. Because the report has no single resource to scope to, the gate must run
// before any OpenCost call is attempted.
func TestCreateDeniedWithoutClusterReadAccess(t *testing.T) {
	fa := &fakeAuthorizer{decision: authorizer.DecisionDeny}
	kc := fake.NewClientBuilder().Build()
	r := NewStorage(kc, fa)

	_, err := r.Create(ctxWithUser(), &costapi.CostReport{}, nil, nil)
	if !apierrors.IsForbidden(err) {
		t.Fatalf("expected Forbidden, got %v", err)
	}
	if fa.got.GetVerb() != "get" || fa.got.GetAPIGroup() != "*" || fa.got.GetResource() != "*" {
		t.Errorf("expected cluster-wide read (get */*), got verb=%q group=%q resource=%q",
			fa.got.GetVerb(), fa.got.GetAPIGroup(), fa.got.GetResource())
	}
}

func TestCreateMissingUserIsBadRequest(t *testing.T) {
	fa := &fakeAuthorizer{decision: authorizer.DecisionAllow}
	kc := fake.NewClientBuilder().Build()
	r := NewStorage(kc, fa)

	_, err := r.Create(context.Background(), &costapi.CostReport{}, nil, nil)
	if !apierrors.IsBadRequest(err) {
		t.Fatalf("expected BadRequest for missing user, got %v", err)
	}
}
