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

package resourceservice

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	uiv1alpha1 "kubeops.dev/ui-server/apis/ui/v1alpha1"
	"kubeops.dev/ui-server/pkg/graph"
	"kubeops.dev/ui-server/pkg/shared"

	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"kmodules.xyz/apiversion"
	kmapi "kmodules.xyz/client-go/api/v1"
	cu "kmodules.xyz/client-go/client"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
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

var _ rest.GroupVersionKindProvider = &Storage{}
var _ rest.Scoper = &Storage{}
var _ rest.Lister = &Storage{}

func NewStorage(kc client.Client, clusterID string, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc:        kc,
		clusterID: clusterID,
		a:         a,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    uiv1alpha1.GroupName,
			Resource: uiv1alpha1.ResourceGenericResourceServices,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return uiv1alpha1.GroupVersion.WithKind(uiv1alpha1.ResourceKindGenericResourceService)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) New() runtime.Object {
	return &uiv1alpha1.GenericResourceService{}
}

func (r *Storage) NewList() runtime.Object {
	return &uiv1alpha1.GenericResourceServiceList{}
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

	items := make([]uiv1alpha1.GenericResourceService, 0)
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

			genres, err := r.toGenericResourceService(item, apiType, cmeta)
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

	result := uiv1alpha1.GenericResourceServiceList{
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

func (r *Storage) toGenericResourceService(item unstructured.Unstructured, apiType *kmapi.ResourceID, cmeta *kmapi.ClusterMetadata) (*uiv1alpha1.GenericResourceService, error) {
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

	genres := uiv1alpha1.GenericResourceService{
		// TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:                       item.GetName(),
			GenerateName:               item.GetGenerateName(),
			Namespace:                  item.GetNamespace(),
			SelfLink:                   "",
			UID:                        item.GetUID(),
			ResourceVersion:            item.GetResourceVersion(),
			Generation:                 item.GetGeneration(),
			CreationTimestamp:          item.GetCreationTimestamp(),
			DeletionTimestamp:          item.GetDeletionTimestamp(),
			DeletionGracePeriodSeconds: item.GetDeletionGracePeriodSeconds(),
			Labels:                     item.GetLabels(),
			Annotations:                item.GetAnnotations(),
			OwnerReferences:            item.GetOwnerReferences(),
			Finalizers:                 item.GetFinalizers(),
			ClusterName:                item.GetClusterName(),
			// ManagedFields:              nil,
		},
		Spec: uiv1alpha1.GenericResourceServiceSpec{
			APIType: *apiType,
			Status: uiv1alpha1.GenericResourceServiceStatus{
				Status:  s.Status.String(),
				Message: s.Message,
			},
		},
		Status: resstatus,
	}
	delete(genres.ObjectMeta.Annotations, "kubectl.kubernetes.io/last-applied-configuration")

	{
		objID := kmapi.NewObjectID(&item)
		oid := objID.OID()

		rid, objs, err := graph.ExecQuery(r.kc, oid, v1alpha1.ResourceLocator{
			Ref: metav1.GroupKind{
				Group: "",
				Kind:  "Service",
			},
			Query: v1alpha1.ResourceQuery{
				Type:    v1alpha1.GraphQLQuery,
				ByLabel: kmapi.EdgeExposedBy,
			},
		})
		if err != nil {
			return nil, err
		}

		genres.Spec.Facilities.Exposed.Usage = uiv1alpha1.FacilityUnknown
		var isExposed bool
		var refs []kmapi.ObjectReference
		for _, obj := range objs {
			var svc core.Service
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &svc); err != nil {
				return nil, err
			}
			if svc.Spec.Type == core.ServiceTypeLoadBalancer ||
				svc.Spec.Type == core.ServiceTypeNodePort ||
				svc.Spec.Type == core.ServiceTypeExternalName {
				isExposed = true
				refs = append(refs, kmapi.ObjectReference{
					Namespace: svc.GetNamespace(),
					Name:      svc.GetName(),
				})
				break
			}
		}
		if isExposed {
			genres.Spec.Facilities.Exposed.Usage = uiv1alpha1.FacilityUsed
			genres.Spec.Facilities.Exposed.Resource = *rid
			genres.Spec.Facilities.Exposed.Refs = refs
		} else {
			genres.Spec.Facilities.Exposed.Usage = uiv1alpha1.FacilityUnused
		}
	}
	{
		objID := kmapi.NewObjectID(&item)
		oid := objID.OID()

		rid, objs, err := graph.ExecQuery(r.kc, oid, v1alpha1.ResourceLocator{
			Ref: metav1.GroupKind{
				Group: "",
				Kind:  "Secret",
			},
			Query: v1alpha1.ResourceQuery{
				Type:    v1alpha1.GraphQLQuery,
				ByLabel: kmapi.EdgeOffshoot,
			},
		})
		if err != nil {
			return nil, err
		}

		genres.Spec.Facilities.TLS.Usage = uiv1alpha1.FacilityUnknown
		var usesTLS bool
		var refs []kmapi.ObjectReference
		for _, obj := range objs {
			if v, ok, _ := unstructured.NestedString(obj.UnstructuredContent(), "type"); ok && v == string(core.SecretTypeTLS) {
				usesTLS = true
				refs = append(refs, kmapi.ObjectReference{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				})
				break
			}
		}
		if usesTLS {
			genres.Spec.Facilities.TLS.Usage = uiv1alpha1.FacilityUsed
			genres.Spec.Facilities.TLS.Resource = *rid
			genres.Spec.Facilities.TLS.Refs = refs
		} else {
			genres.Spec.Facilities.TLS.Usage = uiv1alpha1.FacilityUnused
		}
	}
	{
		objID := kmapi.NewObjectID(&item)
		oid := objID.OID()
		rid, refs, err := graph.ExecRawQuery(r.kc, oid, v1alpha1.ResourceLocator{
			Ref: metav1.GroupKind{
				Group: "stash.appscode.com",
				Kind:  "BackupSession",
			},
			Query: v1alpha1.ResourceQuery{
				Type: v1alpha1.GraphQLQuery,
				Raw: `query Find($src: String!, $targetGroup: String!, $targetKind: String!) {
		         find(oid: $src) {
		           backup_via(group: "stash.appscode.com", kind: "BackupConfiguration") {
		             refs: offshoot(group: $targetGroup, kind: $targetKind) {
		               namespace
		               name
		             }
		           }
		         }
		       }`,
			},
		})
		if err != nil {
			return nil, err
		}

		genres.Spec.Facilities.Backup.Usage = uiv1alpha1.FacilityUnknown
		if len(refs) > 0 {
			genres.Spec.Facilities.Backup.Usage = uiv1alpha1.FacilityUsed
			genres.Spec.Facilities.Backup.Resource = *rid
			genres.Spec.Facilities.Backup.Refs = refs
		} else {
			genres.Spec.Facilities.Backup.Usage = uiv1alpha1.FacilityUnused
		}
	}
	{
		objID := kmapi.NewObjectID(&item)
		oid := objID.OID()

		rid, refs, err := graph.ExecRawQuery(r.kc, oid, v1alpha1.ResourceLocator{
			Ref: metav1.GroupKind{
				Group: "monitoring.coreos.com",
				Kind:  "ServiceMonitor",
			},
			Query: v1alpha1.ResourceQuery{
				Type: v1alpha1.GraphQLQuery,
				Raw: `query Find($src: String!, $targetGroup: String!, $targetKind: String!) {
		           find(oid: $src) {
		             exposed_by(group: "", kind: "Service") {
		               refs: monitored_by(group: $targetGroup, kind: $targetKind) {
		                 namespace
		                 name
		               }
		             }
		           }
		         }`,
			},
		})
		if err != nil {
			return nil, err
		}

		genres.Spec.Facilities.Monitoring.Usage = uiv1alpha1.FacilityUnknown
		if len(refs) > 0 {
			genres.Spec.Facilities.Monitoring.Usage = uiv1alpha1.FacilityUsed
			genres.Spec.Facilities.Monitoring.Resource = *rid
			genres.Spec.Facilities.Monitoring.Refs = refs
		} else {
			rid, refs, err := graph.ExecRawQuery(r.kc, oid, v1alpha1.ResourceLocator{
				Ref: metav1.GroupKind{
					Group: "monitoring.coreos.com",
					Kind:  "PodMonitor",
				},
				Query: v1alpha1.ResourceQuery{
					Type: v1alpha1.GraphQLQuery,
					Raw: `query Find($src: String!, $targetGroup: String!, $targetKind: String!) {
		           find(oid: $src) {
		             exposed_by(group: "", kind: "Service") {
		               refs: monitored_by(group: $targetGroup, kind: $targetKind) {
		                 namespace
		                 name
		               }
		             }
		           }
		         }`,
				},
			})
			if err != nil {
				return nil, err
			}

			genres.Spec.Facilities.Monitoring.Usage = uiv1alpha1.FacilityUnknown
			if len(refs) > 0 {
				genres.Spec.Facilities.Monitoring.Usage = uiv1alpha1.FacilityUsed
				genres.Spec.Facilities.Monitoring.Resource = *rid
				genres.Spec.Facilities.Monitoring.Refs = refs
			} else {
				genres.Spec.Facilities.Monitoring.Usage = uiv1alpha1.FacilityUnused
			}
		}
	}

	return &genres, nil
}
