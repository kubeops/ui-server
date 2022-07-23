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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"kubeops.dev/ui-server/pkg/graph"
	"kubeops.dev/ui-server/pkg/shared"

	"github.com/pkg/errors"
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
	mu "kmodules.xyz/client-go/meta"
	corev1alpha1 "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	sharedapi "kmodules.xyz/resource-metadata/apis/shared"
	"kmodules.xyz/resource-metadata/hub/resourcedescriptors"
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
			Resource: corev1alpha1.ResourceGenericResourceServices,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return corev1alpha1.GroupVersion.WithKind(corev1alpha1.ResourceKindGenericResourceService)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) New() runtime.Object {
	return &corev1alpha1.GenericResourceService{}
}

func (r *Storage) NewList() runtime.Object {
	return &corev1alpha1.GenericResourceServiceList{}
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

	return r.toGenericResourceService(obj, rid, cmeta)
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

	items := make([]corev1alpha1.GenericResourceService, 0)
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

	result := corev1alpha1.GenericResourceServiceList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    items,
	}

	return &result, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func (r *Storage) toGenericResourceService(item unstructured.Unstructured, apiType *kmapi.ResourceID, cmeta *kmapi.ClusterMetadata) (*corev1alpha1.GenericResourceService, error) {
	content := item.UnstructuredContent()

	objID := kmapi.NewObjectID(&item)
	oid := objID.OID()

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

	genres := corev1alpha1.GenericResourceService{
		// TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:                       corev1alpha1.GetGenericResourceName(&item),
			GenerateName:               item.GetGenerateName(),
			Namespace:                  item.GetNamespace(),
			SelfLink:                   "",
			UID:                        "gsvc-" + item.GetUID(),
			ResourceVersion:            item.GetResourceVersion(),
			Generation:                 item.GetGeneration(),
			CreationTimestamp:          item.GetCreationTimestamp(),
			DeletionTimestamp:          item.GetDeletionTimestamp(),
			DeletionGracePeriodSeconds: item.GetDeletionGracePeriodSeconds(),
			Labels:                     item.GetLabels(),
			Annotations:                map[string]string{},
			// OwnerReferences:            item.GetOwnerReferences(),
			// Finalizers:                 item.GetFinalizers(),
			ZZZ_DeprecatedClusterName: item.GetZZZ_DeprecatedClusterName(),
			// ManagedFields:              nil,
		},
		Spec: corev1alpha1.GenericResourceServiceSpec{
			Cluster: *cmeta,
			APIType: *apiType,
			Name:    item.GetName(),
			Status: corev1alpha1.GenericResourceServiceStatus{
				Status:  s.Status.String(),
				Message: s.Message,
			},
			Facilities: corev1alpha1.GenericResourceServiceFacilities{
				Exposed: corev1alpha1.GenericResourceServiceFacilitator{
					Usage: corev1alpha1.FacilityUnknown,
				},
				TLS: corev1alpha1.GenericResourceServiceFacilitator{
					Usage: corev1alpha1.FacilityUnknown,
				},
				Backup: corev1alpha1.GenericResourceServiceFacilitator{
					Usage: corev1alpha1.FacilityUnknown,
				},
				Monitoring: corev1alpha1.GenericResourceServiceFacilitator{
					Usage: corev1alpha1.FacilityUnknown,
				},
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
		rid, objs, err := graph.ExecQuery(r.kc, oid, sharedapi.ResourceLocator{
			Ref: metav1.GroupKind{
				Group: "",
				Kind:  "Service",
			},
			Query: sharedapi.ResourceQuery{
				Type:    sharedapi.GraphQLQuery,
				ByLabel: kmapi.EdgeExposedBy,
			},
		})
		if err != nil {
			return nil, err
		}

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
			genres.Spec.Facilities.Exposed.Usage = corev1alpha1.FacilityUsed
			genres.Spec.Facilities.Exposed.Resource = rid
			genres.Spec.Facilities.Exposed.Refs = refs
		} else {
			genres.Spec.Facilities.Exposed.Usage = corev1alpha1.FacilityUnused
		}
	}
	{
		yes, err := resourcemetrics.UsesTLS(content)
		if err != nil {
			return nil, err
		}
		if yes {
			genres.Spec.Facilities.TLS.Usage = corev1alpha1.FacilityUsed
		} else {
			genres.Spec.Facilities.TLS.Usage = corev1alpha1.FacilityUnused
		}
	}
	{
		rid, refs, err := graph.ExecRawQuery(r.kc, oid, sharedapi.ResourceLocator{
			Ref: metav1.GroupKind{
				Group: "stash.appscode.com",
				Kind:  "BackupSession",
			},
			Query: sharedapi.ResourceQuery{
				Type: sharedapi.GraphQLQuery,
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
		if err == nil {
			if len(refs) > 0 {
				genres.Spec.Facilities.Backup.Usage = corev1alpha1.FacilityUsed
				genres.Spec.Facilities.Backup.Resource = rid
				genres.Spec.Facilities.Backup.Refs = refs
			} else {
				genres.Spec.Facilities.Backup.Usage = corev1alpha1.FacilityUnused
			}
		} else if !meta.IsNoMatchError(err) {
			return nil, err
		}
	}
	{
		rid, refs, err := graph.ExecRawQuery(r.kc, oid, sharedapi.ResourceLocator{
			Ref: metav1.GroupKind{
				Group: "monitoring.coreos.com",
				Kind:  "ServiceMonitor",
			},
			Query: sharedapi.ResourceQuery{
				Type: sharedapi.GraphQLQuery,
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
		if err == nil {
			if len(refs) > 0 {
				genres.Spec.Facilities.Monitoring.Usage = corev1alpha1.FacilityUsed
				genres.Spec.Facilities.Monitoring.Resource = rid
				genres.Spec.Facilities.Monitoring.Refs = refs
			}
		} else if !meta.IsNoMatchError(err) {
			return nil, err
		}

		if genres.Spec.Facilities.Monitoring.Usage == corev1alpha1.FacilityUnknown {
			rid, refs, err = graph.ExecRawQuery(r.kc, oid, sharedapi.ResourceLocator{
				Ref: metav1.GroupKind{
					Group: "monitoring.coreos.com",
					Kind:  "PodMonitor",
				},
				Query: sharedapi.ResourceQuery{
					Type: sharedapi.GraphQLQuery,
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
			if err == nil {
				if len(refs) > 0 {
					genres.Spec.Facilities.Monitoring.Usage = corev1alpha1.FacilityUsed
					genres.Spec.Facilities.Monitoring.Resource = rid
					genres.Spec.Facilities.Monitoring.Refs = refs
				} else {
					genres.Spec.Facilities.Monitoring.Usage = corev1alpha1.FacilityUnused
				}
			} else if !meta.IsNoMatchError(err) {
				return nil, err
			}
		}
	}
	{
		buf := shared.BufferPool.Get().(*bytes.Buffer)
		defer shared.BufferPool.Put(buf)

		gvr := apiType.GroupVersionResource()
		podGVR := schema.GroupVersionResource{Version: "v1", Resource: "Pods"}
		if rd, err := resourcedescriptors.LoadByGVR(gvr); err == nil {
			execServices := make([]corev1alpha1.ExecServiceFacilitator, 0, len(rd.Spec.Exec))

			for _, exec := range rd.Spec.Exec {
				cond := true
				if exec.If != nil {
					if exec.If.Condition != "" {
						buf.Reset()
						result, err := shared.RenderTemplate(exec.If.Condition, content, buf)
						if err != nil {
							return nil, errors.Wrapf(err, "failed to check condition for %+v exec with alias %s", gvr, exec.Alias)
						}
						result = strings.TrimSpace(result)
						cond = strings.EqualFold(result, "true")
					} else if exec.If.Connected != nil {
						_, targets, err := graph.ExecRawQuery(r.kc, oid, *exec.If.Connected)
						if err != nil {
							return nil, errors.Wrapf(err, "failed to check connection for %+v exec with alias %s", gvr, exec.Alias)
						}
						cond = len(targets) > 0
					}
				}
				if !cond {
					continue
				}

				if gvr == podGVR {
					execServices = append(execServices, corev1alpha1.ExecServiceFacilitator{
						Alias: exec.Alias,
						Resource: kmapi.ResourceID{
							Group:   "",
							Version: "v1",
							Name:    "pods",
							Kind:    "Pod",
							Scope:   kmapi.NamespaceScoped,
						},
						Ref: kmapi.ObjectReference{
							Namespace: item.GetNamespace(),
							Name:      item.GetName(),
						},
						Container:      exec.Container,
						Command:        exec.Command,
						Help:           exec.Help,
						KubectlCommand: genKubectlCommand("Pod", item.GetName(), item.GetNamespace(), exec),
					})
				} else {
					buf.Reset()
					svcName, err := shared.RenderTemplate(exec.ServiceNameTemplate, content, buf)
					if err != nil {
						return nil, errors.Wrapf(err, "failed to render service name for %+v exec with alias %s", gvr, exec.Alias)
					}

					execServices = append(execServices, corev1alpha1.ExecServiceFacilitator{
						Alias: exec.Alias,
						Resource: kmapi.ResourceID{
							Group:   "",
							Version: "v1",
							Name:    "services",
							Kind:    "Service",
							Scope:   kmapi.NamespaceScoped,
						},
						Ref: kmapi.ObjectReference{
							Namespace: item.GetNamespace(),
							Name:      svcName,
						},
						Container:      exec.Container,
						Command:        exec.Command,
						Help:           exec.Help,
						KubectlCommand: genKubectlCommand("Service", svcName, item.GetNamespace(), exec),
					})
				}
			}

			genres.Spec.Facilities.Exec = execServices
		}
	}

	return &genres, nil
}

func genKubectlCommand(kind, name, ns string, exec rsapi.ResourceExec) string {
	isBash := func(cmd []string) bool {
		return len(cmd) > 2 &&
			(cmd[0] == "bash" || cmd[0] == "/bin/bash" || cmd[0] == "sh" || cmd[0] == "/bin/sh") &&
			cmd[1] == "-c"
	}
	cmd := fmt.Sprintf("kubectl exec -it -n %s %s/%s", ns, strings.ToLower(kind), name)
	if exec.Container != "" {
		cmd += fmt.Sprintf("  -c %s", exec.Container)
	}
	if isBash(exec.Command) {
		cmd += fmt.Sprintf(" -- %s %s '%s'", exec.Command[0], exec.Command[1], exec.Command[2])
	} else {
		cmd += fmt.Sprintf(" -- %s", strings.Join(exec.Command, " "))
	}
	return cmd
}
