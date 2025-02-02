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

package selfsubjectnamespaceaccessreview

import (
	"context"
	"sort"

	authorization "k8s.io/api/authorization/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes"
	kmapi "kmodules.xyz/client-go/api/v1"
	clustermeta "kmodules.xyz/client-go/cluster"
	identityapi "kmodules.xyz/resource-metadata/apis/identity/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc  kubernetes.Interface
	rtc client.Client
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc kubernetes.Interface, rtc client.Client) *Storage {
	return &Storage{
		kc:  kc,
		rtc: rtc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return identityapi.SchemeGroupVersion.WithKind(identityapi.ResourceKindSelfSubjectNamespaceAccessReview)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return identityapi.ResourceSelfSubjectNamespaceAccessReview
}

func (r *Storage) New() runtime.Object {
	return &identityapi.SelfSubjectNamespaceAccessReview{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*identityapi.SelfSubjectNamespaceAccessReview)
	if in.Name != "" {
		return in, apierrors.NewBadRequest("metadata.name must be empty")
	}

	user, ok := request.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}
	extra := make(map[string]authorization.ExtraValue)
	for k, v := range user.GetExtra() {
		extra[k] = v
	}

	var allNs []core.Namespace

	orgId, found := user.GetExtra()[kmapi.AceOrgIDKey]
	if !found || len(orgId) == 0 || len(orgId) > 1 {
		var list core.NamespaceList
		err := r.rtc.List(ctx, &list)
		if err != nil {
			return nil, err
		}
		allNs = list.Items
	} else {
		// for client org users, only consider client org ns
		var list core.NamespaceList
		err := r.rtc.List(ctx, &list, client.MatchingLabels{
			kmapi.ClientOrgKey: "true",
		})
		if err != nil {
			return nil, err
		}
		for _, ns := range list.Items {
			if ns.Annotations[kmapi.AceOrgIDKey] == orgId[0] {
				allNs = append(allNs, ns)
			}
		}
	}

	allowedNs := make([]core.Namespace, 0, len(allNs))
	for _, ns := range allNs {
		allowed, err := r.hasNamespaceResourceAccess(ctx, in, ns.Name, user, extra)
		if err != nil {
			return nil, err
		}
		if allowed {
			allowedNs = append(allowedNs, ns)
		}
	}

	// check for all namespaces
	{
		allowed, err := r.hasAllNamespaceResourceAccess(ctx, in, user, extra)
		if err != nil {
			return nil, err
		}
		if allowed {
			allowed, err = r.hasNonResourceAccess(ctx, in, user, extra)
			if err != nil {
				return nil, err
			}
		}
		in.Status.AllNamespaces = allowed
	}

	if clustermeta.IsRancherManaged(r.rtc.RESTMapper()) {
		projects := map[string][]string{}
		for _, ns := range allowedNs {
			projectId, exists := ns.Labels[clustermeta.LabelKeyRancherFieldProjectId]
			if !exists {
				projectId = clustermeta.FakeRancherProjectId
			}
			projects[projectId] = append(projects[projectId], ns.Name)
		}

		for projectId, namespaces := range projects {
			sort.Strings(namespaces)
			projects[projectId] = namespaces
		}
		in.Status.Projects = projects
	} else {
		namespaces := make([]string, 0, len(allowedNs))
		for _, ns := range allowedNs {
			namespaces = append(namespaces, ns.Name)
		}

		sort.Strings(namespaces)
		in.Status.Namespaces = namespaces
	}

	return in, nil
}

func (r *Storage) hasNonResourceAccess(ctx context.Context, in *identityapi.SelfSubjectNamespaceAccessReview, user user.Info, extra map[string]authorization.ExtraValue) (bool, error) {
	for _, attr := range in.Spec.NonResourceAttributes {
		review := &authorization.SubjectAccessReview{
			Spec: authorization.SubjectAccessReviewSpec{
				NonResourceAttributes: &attr,
				User:                  user.GetName(),
				Groups:                user.GetGroups(),
				Extra:                 extra,
				UID:                   user.GetUID(),
			},
		}
		review, err := r.kc.AuthorizationV1().SubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}
		if !review.Status.Allowed {
			return false, nil
		}
	}
	return true, nil
}

func (r *Storage) hasAllNamespaceResourceAccess(ctx context.Context, in *identityapi.SelfSubjectNamespaceAccessReview, user user.Info, extra map[string]authorization.ExtraValue) (bool, error) {
	for _, attr := range in.Spec.ResourceAttributes {
		attr.Namespace = ""
		review := &authorization.SubjectAccessReview{
			Spec: authorization.SubjectAccessReviewSpec{
				ResourceAttributes: &attr,
				User:               user.GetName(),
				Groups:             user.GetGroups(),
				Extra:              extra,
				UID:                user.GetUID(),
			},
		}
		review, err := r.kc.AuthorizationV1().SubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}
		if !review.Status.Allowed {
			return false, nil
		}
	}
	return true, nil
}

func (r *Storage) hasNamespaceResourceAccess(ctx context.Context, in *identityapi.SelfSubjectNamespaceAccessReview, ns string, user user.Info, extra map[string]authorization.ExtraValue) (bool, error) {
	for _, attr := range in.Spec.ResourceAttributes {
		attr.Namespace = ns
		review := &authorization.LocalSubjectAccessReview{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
			},
			Spec: authorization.SubjectAccessReviewSpec{
				ResourceAttributes:    &attr,
				NonResourceAttributes: nil,
				User:                  user.GetName(),
				Groups:                user.GetGroups(),
				Extra:                 extra,
				UID:                   user.GetUID(),
			},
		}
		review, err := r.kc.AuthorizationV1().LocalSubjectAccessReviews(ns).Create(ctx, review, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}
		if !review.Status.Allowed {
			return false, nil
		}
	}
	return true, nil
}
