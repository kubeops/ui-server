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

	identityapi "kubeops.dev/ui-server/apis/identity/v1alpha1"

	authorization "k8s.io/api/authorization/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes"
	clustermeta "kmodules.xyz/client-go/cluster"
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
	return identityapi.GroupVersion.WithKind(identityapi.ResourceKindSelfSubjectNamespaceAccessReview)
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

	var list core.NamespaceList
	err := r.rtc.List(ctx, &list)
	if err != nil {
		return nil, err
	}

	allowedNs := make([]core.Namespace, 0, len(list.Items))
	for _, ns := range list.Items {
		allowed := true

		for _, attr := range in.Spec.ResourceAttributes {
			attr.Namespace = ns.Name
			review := &authorization.LocalSubjectAccessReview{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns.Name,
				},
				Spec: authorization.SubjectAccessReviewSpec{
					ResourceAttributes:    &attr,
					NonResourceAttributes: nil,
					User:                  user.GetName(),
					Groups:                user.GetGroups(),
					Extra:                 extra,
					UID:                   user.GetName(),
				},
			}
			review, err = r.kc.AuthorizationV1().LocalSubjectAccessReviews(ns.Name).Create(ctx, review, metav1.CreateOptions{})
			if err != nil {
				return nil, err
			}
			if !review.Status.Allowed {
				allowed = false
				break
			}
		}
		for _, attr := range in.Spec.NonResourceAttributes {
			review := &authorization.LocalSubjectAccessReview{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns.Name,
				},
				Spec: authorization.SubjectAccessReviewSpec{
					ResourceAttributes:    nil,
					NonResourceAttributes: &attr,
					User:                  user.GetName(),
					Groups:                user.GetGroups(),
					Extra:                 extra,
					UID:                   user.GetName(),
				},
			}
			review, err = r.kc.AuthorizationV1().LocalSubjectAccessReviews(ns.Name).Create(ctx, review, metav1.CreateOptions{})
			if err != nil {
				return nil, err
			}
			if !review.Status.Allowed {
				allowed = false
				break
			}
		}

		if allowed {
			allowedNs = append(allowedNs, ns)
		}
	}

	if clustermeta.IsRancherManaged(r.rtc.RESTMapper()) {
		projects := map[string][]string{}
		for _, ns := range allowedNs {
			projectId := ns.Labels[clustermeta.LabelKeyRancherFieldProjectId]
			projects[projectId] = append(projects[projectId], ns.Name)
		}

		for projectId, namespaces := range projects {
			sort.Strings(namespaces)
			projects[projectId] = namespaces
		}
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
