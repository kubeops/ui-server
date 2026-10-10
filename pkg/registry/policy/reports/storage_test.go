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

	policyapi "kubeops.dev/ui-server/apis/policy/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	kmapi "kmodules.xyz/client-go/api/v1"
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

// TestAuthorizeScopes verifies that each PolicyReport scope is gated against the expected
// access before any violation data is read. The authorizer denies, so Create returns
// Forbidden and we assert on the attributes it was asked about.
func TestAuthorizeScopes(t *testing.T) {
	cases := []struct {
		name         string
		req          *policyapi.PolicyReportRequest
		wantVerb     string
		wantAPIGroup string
		wantResource string
		wantNS       string
		wantName     string
	}{
		{
			name:         "cluster",
			req:          nil,
			wantVerb:     "get",
			wantAPIGroup: "*",
			wantResource: "*",
		},
		{
			name:         "namespace",
			req:          &policyapi.PolicyReportRequest{ObjectInfo: kmapi.ObjectInfo{Resource: kmapi.ResourceID{Name: "namespaces"}, Ref: kmapi.ObjectReference{Name: "ns1"}}},
			wantVerb:     "get",
			wantResource: "namespaces",
			wantName:     "ns1",
		},
		{
			name:         "resource",
			req:          &policyapi.PolicyReportRequest{ObjectInfo: kmapi.ObjectInfo{Resource: kmapi.ResourceID{Group: "apps", Kind: "Deployment"}, Ref: kmapi.ObjectReference{Namespace: "ns3", Name: "dep1"}}},
			wantVerb:     "get",
			wantAPIGroup: "apps",
			wantResource: "deployments",
			wantNS:       "ns3",
			wantName:     "dep1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fa := &fakeAuthorizer{decision: authorizer.DecisionDeny}
			kc := fake.NewClientBuilder().WithRESTMapper(testMapper()).Build()
			r := NewStorage(kc, fa)

			in := &policyapi.PolicyReport{Request: tc.req}
			_, err := r.Create(ctxWithUser(), in, nil, nil)
			if !apierrors.IsForbidden(err) {
				t.Fatalf("expected Forbidden, got %v", err)
			}
			if got := fa.got.GetVerb(); got != tc.wantVerb {
				t.Errorf("verb: got %q want %q", got, tc.wantVerb)
			}
			if got := fa.got.GetAPIGroup(); got != tc.wantAPIGroup {
				t.Errorf("apiGroup: got %q want %q", got, tc.wantAPIGroup)
			}
			if got := fa.got.GetResource(); got != tc.wantResource {
				t.Errorf("resource: got %q want %q", got, tc.wantResource)
			}
			if got := fa.got.GetNamespace(); got != tc.wantNS {
				t.Errorf("namespace: got %q want %q", got, tc.wantNS)
			}
			if got := fa.got.GetName(); got != tc.wantName {
				t.Errorf("name: got %q want %q", got, tc.wantName)
			}
		})
	}
}
