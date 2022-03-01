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

package genericresource

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"kubeops.dev/ui-server/pkg/shared"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"kmodules.xyz/apiversion"
	kmapi "kmodules.xyz/client-go/api/v1"
	cu "kmodules.xyz/client-go/client"
	mu "kmodules.xyz/client-go/meta"
	corev1alpha1 "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
	resourcemetrics "kmodules.xyz/resource-metrics"
	"kmodules.xyz/resource-metrics/api"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc        client.Client
	clusterID string
	a         authorizer.Authorizer
	convertor rest.TableConvertor
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Getter                   = &Storage{}
	_ rest.Lister                   = &Storage{}
)

func NewStorage(kc client.Client, clusterID string, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc:        kc,
		clusterID: clusterID,
		a:         a,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    corev1alpha1.GroupName,
			Resource: corev1alpha1.ResourceGenericResources,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return corev1alpha1.GroupVersion.WithKind(corev1alpha1.ResourceKindGenericResource)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) New() runtime.Object {
	return &corev1alpha1.GenericResource{}
}

func (r *Storage) NewList() runtime.Object {
	return &corev1alpha1.GenericResourceList{}
}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	cmeta, err := cu.ClusterMetadata(r.kc)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	objName, gk, err := corev1alpha1.ParseGenericResourceName(name)
	if err != nil {
		return nil, apierrors.NewBadRequest(err.Error())
	}
	mapping, err := r.kc.RESTMapper().RESTMapping(gk)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	rid := kmapi.NewResourceID(mapping)

	attrs := authorizer.AttributesRecord{
		User:      user,
		Verb:      "get",
		Namespace: ns,
		APIGroup:  mapping.Resource.Group,
		Resource:  mapping.Resource.Resource,
		Name:      objName,
	}
	decision, why, err := r.a.Authorize(ctx, attrs)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	if decision != authorizer.DecisionAllow {
		return nil, apierrors.NewForbidden(mapping.Resource.GroupResource(), objName, errors.New(why))
	}

	var obj unstructured.Unstructured
	obj.SetGroupVersionKind(mapping.GroupVersionKind)
	err = r.kc.Get(ctx, client.ObjectKey{Namespace: ns, Name: objName}, &obj)
	if err != nil {
		return nil, err
	}

	return r.toGenericResource(obj, rid, cmeta)
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	ns, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing namespace")
	}

	selector := shared.NewGroupKindSelector(options.LabelSelector)

	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("missing user info")
	}

	cmeta, err := cu.ClusterMetadata(r.kc)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	items := make([]corev1alpha1.GenericResource, 0)
	for _, gvk := range api.RegisteredTypes() {
		if !selector.Matches(gvk.GroupKind()) {
			continue
		}

		mapping, err := r.kc.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
		if meta.IsNoMatchError(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		apiType := kmapi.NewResourceID(mapping)

		attrs := authorizer.AttributesRecord{
			User:      user,
			Verb:      "get",
			Namespace: ns,
			APIGroup:  mapping.Resource.Group,
			Resource:  mapping.Resource.Resource,
			Name:      "",
		}

		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(gvk)
		if err := r.kc.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			attrs.Name = item.GetName()
			decision, _, err := r.a.Authorize(ctx, attrs)
			if err != nil {
				return nil, apierrors.NewInternalError(err)
			}
			if decision != authorizer.DecisionAllow {
				continue
			}

			genres, err := r.toGenericResource(item, apiType, cmeta)
			if err != nil {
				return nil, err
			}
			items = append(items, *genres)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		gvk_i := items[i].GetObjectKind().GroupVersionKind()
		gvk_j := items[j].GetObjectKind().GroupVersionKind()
		if gvk_i.Group != gvk_j.Group {
			return gvk_i.Group < gvk_j.Group
		}
		if gvk_i.Version != gvk_j.Version {
			diff, _ := apiversion.Compare(gvk_i.Version, gvk_j.Version)
			return diff < 0
		}
		if gvk_i.Kind != gvk_j.Kind {
			return gvk_i.Kind < gvk_j.Kind
		}
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
	})

	result := corev1alpha1.GenericResourceList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    items,
	}
	result.ListMeta.SelfLink = ""

	return &result, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func (r *Storage) toGenericResource(item unstructured.Unstructured, apiType *kmapi.ResourceID, cmeta *kmapi.ClusterMetadata) (*corev1alpha1.GenericResource, error) {
	content := item.UnstructuredContent()

	s, err := status.Compute(&item)
	if err != nil {
		return nil, err
	}

	var resstatus *runtime.RawExtension
	if v, ok, _ := unstructured.NestedFieldNoCopy(content, "status"); ok {
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to convert status to json, reason: %v", err)
		}
		resstatus = &runtime.RawExtension{Raw: data}
	}

	genres := corev1alpha1.GenericResource{
		// TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:                       corev1alpha1.GetGenericResourceName(&item),
			GenerateName:               item.GetGenerateName(),
			Namespace:                  item.GetNamespace(),
			SelfLink:                   "",
			UID:                        types.UID(uuid.Must(uuid.NewUUID()).String()),
			ResourceVersion:            item.GetResourceVersion(),
			Generation:                 item.GetGeneration(),
			CreationTimestamp:          item.GetCreationTimestamp(),
			DeletionTimestamp:          item.GetDeletionTimestamp(),
			DeletionGracePeriodSeconds: item.GetDeletionGracePeriodSeconds(),
			Labels:                     item.GetLabels(),
			Annotations:                map[string]string{},
			// OwnerReferences:            item.GetOwnerReferences(),
			// Finalizers:                 item.GetFinalizers(),
			ClusterName: item.GetClusterName(),
			// ManagedFields:              nil,
		},
		Spec: corev1alpha1.GenericResourceSpec{
			Cluster:              *cmeta,
			APIType:              *apiType,
			Name:                 item.GetName(),
			Replicas:             0,
			RoleReplicas:         nil,
			Mode:                 "",
			TotalResource:        core.ResourceRequirements{},
			AppResource:          core.ResourceRequirements{},
			RoleResourceLimits:   nil,
			RoleResourceRequests: nil,

			Status: corev1alpha1.GenericResourceStatus{
				Status:  s.Status.String(),
				Message: s.Message,
			},
		},
		Status: resstatus,
	}
	for k, v := range item.GetAnnotations() {
		if k != mu.LastAppliedConfigAnnotation {
			genres.Annotations[k] = v
		}
	}

	{
		if v, ok, _ := unstructured.NestedString(item.UnstructuredContent(), "spec", "version"); ok {
			genres.Spec.Version = v
		}
	}
	{
		rv, err := resourcemetrics.Replicas(content)
		if err != nil {
			return nil, err
		}
		genres.Spec.Replicas = rv
	}
	{
		rv, err := resourcemetrics.RoleReplicas(content)
		if err != nil {
			return nil, err
		}
		genres.Spec.RoleReplicas = rv
	}
	{
		rv, err := resourcemetrics.Mode(content)
		if err != nil {
			return nil, err
		}
		genres.Spec.Mode = rv
	}
	{
		rv, err := resourcemetrics.TotalResourceRequests(content)
		if err != nil {
			return nil, err
		}
		genres.Spec.TotalResource.Requests = rv
	}
	{
		rv, err := resourcemetrics.TotalResourceLimits(content)
		if err != nil {
			return nil, err
		}
		genres.Spec.TotalResource.Limits = rv
	}
	{
		rv, err := resourcemetrics.AppResourceRequests(content)
		if err != nil {
			return nil, err
		}
		genres.Spec.AppResource.Requests = rv
	}
	{
		rv, err := resourcemetrics.AppResourceLimits(content)
		if err != nil {
			return nil, err
		}
		genres.Spec.AppResource.Limits = rv
	}
	{
		rv, err := resourcemetrics.RoleResourceRequests(content)
		if err != nil {
			return nil, err
		}
		genres.Spec.RoleResourceRequests = rv
	}
	{
		rv, err := resourcemetrics.RoleResourceLimits(content)
		if err != nil {
			return nil, err
		}
		genres.Spec.RoleResourceLimits = rv
	}
	return &genres, nil
}
