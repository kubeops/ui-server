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

package authorizer

import (
	"context"

	"kmodules.xyz/authorizer/apiserver"
	"kmodules.xyz/authorizer/rbac"

	"k8s.io/apiserver/pkg/authorization/authorizer"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type combined struct {
	rbac authorizer.Authorizer
	api  authorizer.Authorizer
}

var _ authorizer.Authorizer = &combined{}

func NewForManagerOrDie(ctx context.Context, mgr manager.Manager) authorizer.Authorizer {
	return &combined{
		rbac: rbac.NewForManagerOrDie(ctx, mgr),
		api:  apiserver.New(mgr.GetClient()),
	}
}

func (c combined) Authorize(ctx context.Context, attrs authorizer.Attributes) (authorized authorizer.Decision, reason string, err error) {
	d, r, err := c.rbac.Authorize(ctx, attrs)
	if err == nil && d == authorizer.DecisionNoOpinion {
		return c.api.Authorize(ctx, attrs)
	}
	return d, r, err
}
