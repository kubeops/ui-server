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

package project

import (
	"context"
	"time"

	"github.com/google/uuid"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/registry/rest"
	clustermeta "kmodules.xyz/client-go/cluster"
	corev1alpha1 "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Client
	convertor rest.TableConvertor
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Lister                   = &Storage{}
	_ rest.Getter                   = &Storage{}

	gr = schema.GroupResource{
		Group:    corev1alpha1.GroupName,
		Resource: corev1alpha1.ResourceProjects,
	}
)

func NewStorage(kc client.Client) *Storage {
	s := &Storage{
		kc:        kc,
		convertor: rest.NewDefaultTableConvertor(gr),
	}
	return s
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return corev1alpha1.SchemeGroupVersion.WithKind(corev1alpha1.ResourceKindProject)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &corev1alpha1.Project{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	if clustermeta.IsRancherManaged(r.kc.RESTMapper()) {
		return GetRancherProject(r.kc, name)
	}
	return nil, apierrors.NewNotFound(gr, name)
}

func (r *Storage) NewList() runtime.Object {
	return &corev1alpha1.ProjectList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	var projects []corev1alpha1.Project
	var err error
	if clustermeta.IsRancherManaged(r.kc.RESTMapper()) {
		projects, err = ListRancherProjects(r.kc)
		if err != nil {
			return nil, err
		}
	}

	result := corev1alpha1.ProjectList{
		TypeMeta: metav1.TypeMeta{},
		// ListMeta: nil,
		Items: projects,
	}

	return &result, err
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func ListRancherProjects(kc client.Client) ([]corev1alpha1.Project, error) {
	var list core.NamespaceList
	err := kc.List(context.TODO(), &list)
	if meta.IsNoMatchError(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	projects := map[string]corev1alpha1.Project{}
	now := time.Now()
	for _, ns := range list.Items {
		projectId, exists := ns.Labels[clustermeta.LabelKeyRancherProjectId]
		if !exists {
			continue
		}

		project, exists := projects[projectId]
		if !exists {
			project = corev1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:              projectId,
					CreationTimestamp: metav1.NewTime(now),
					UID:               types.UID(uuid.Must(uuid.NewUUID()).String()),
					Labels: map[string]string{
						clustermeta.LabelKeyRancherProjectId: projectId,
					},
				},
				Spec: corev1alpha1.ProjectSpec{
					Type:       corev1alpha1.ProjectUser,
					Namespaces: nil,
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							clustermeta.LabelKeyRancherProjectId: projectId,
						},
					},
				},
			}
		}

		if ns.CreationTimestamp.Before(&project.CreationTimestamp) {
			project.CreationTimestamp = ns.CreationTimestamp
		}

		if ns.Name == metav1.NamespaceDefault {
			project.Spec.Type = corev1alpha1.ProjectDefault
		} else if ns.Name == metav1.NamespaceSystem {
			project.Spec.Type = corev1alpha1.ProjectSystem
		}
		project.Spec.Namespaces = append(project.Spec.Namespaces, ns.Name)

		projects[projectId] = project
	}

	result := make([]corev1alpha1.Project, 0, len(projects))
	for _, p := range projects {
		result = append(result, p)
	}
	return result, nil
}

func GetRancherProject(kc client.Client, projectId string) (*corev1alpha1.Project, error) {
	var list core.NamespaceList
	err := kc.List(context.TODO(), &list, client.MatchingLabels{
		clustermeta.LabelKeyRancherProjectId: projectId,
	})
	if err != nil {
		return nil, err
	} else if len(list.Items) == 0 {
		return nil, apierrors.NewNotFound(gr, projectId)
	}

	now := time.Now()
	project := corev1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:              projectId,
			CreationTimestamp: metav1.NewTime(now),
			UID:               types.UID(uuid.Must(uuid.NewUUID()).String()),
			Labels: map[string]string{
				clustermeta.LabelKeyRancherProjectId: projectId,
			},
		},
		Spec: corev1alpha1.ProjectSpec{
			Type:       corev1alpha1.ProjectUser,
			Namespaces: nil,
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					clustermeta.LabelKeyRancherProjectId: projectId,
				},
			},
		},
	}
	for _, ns := range list.Items {
		if ns.CreationTimestamp.Before(&project.CreationTimestamp) {
			project.CreationTimestamp = ns.CreationTimestamp
		}

		if ns.Name == metav1.NamespaceDefault {
			project.Spec.Type = corev1alpha1.ProjectDefault
		} else if ns.Name == metav1.NamespaceSystem {
			project.Spec.Type = corev1alpha1.ProjectSystem
		}
		project.Spec.Namespaces = append(project.Spec.Namespaces, ns.Name)
	}

	return &project, nil
}
