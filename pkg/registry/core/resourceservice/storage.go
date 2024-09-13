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
	catalogapi "go.bytebuilders.dev/catalog/api/v1alpha1"
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
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"kmodules.xyz/apiversion"
	kmapi "kmodules.xyz/client-go/api/v1"
	clustermeta "kmodules.xyz/client-go/cluster"
	mu "kmodules.xyz/client-go/meta"
	ofst "kmodules.xyz/offshoot-api/api/v1"
	rscoreapi "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
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
	dc        discovery.DiscoveryInterface
	clusterID string
	a         authorizer.Authorizer
	convertor rest.TableConvertor
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Getter                   = &Storage{}
	_ rest.Lister                   = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client, dc discovery.DiscoveryInterface, clusterID string, a authorizer.Authorizer) *Storage {
	return &Storage{
		kc:        kc,
		dc:        dc,
		clusterID: clusterID,
		a:         a,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    rscoreapi.GroupName,
			Resource: rscoreapi.ResourceGenericResourceServices,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rscoreapi.SchemeGroupVersion.WithKind(rscoreapi.ResourceKindGenericResourceService)
}

func (r *Storage) NamespaceScoped() bool {
	return true
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rscoreapi.ResourceKindGenericResourceService)
}

func (r *Storage) New() runtime.Object {
	return &rscoreapi.GenericResourceService{}
}

func (r *Storage) Destroy() {}

func (r *Storage) NewList() runtime.Object {
	return &rscoreapi.GenericResourceServiceList{}
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

	cmeta, err := clustermeta.ClusterMetadata(r.kc)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	objName, gk, err := rscoreapi.ParseGenericResourceName(name)
	if err != nil {
		return nil, apierrors.NewBadRequest(err.Error())
	}
	mapping, err := r.kc.RESTMapper().RESTMapping(gk)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	rid := kmapi.NewResourceID(mapping)

	attrs := authorizer.AttributesRecord{
		User:            user,
		Verb:            "get",
		Namespace:       ns,
		APIGroup:        mapping.Resource.Group,
		Resource:        mapping.Resource.Resource,
		Name:            objName,
		ResourceRequest: true,
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

	cmeta, err := clustermeta.ClusterMetadata(r.kc)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(r.dc))
	gvks := make(map[schema.GroupKind]string)
	for _, gvk := range api.RegisteredTypes() {
		if !selector.Matches(gvk.GroupKind()) {
			continue
		}
		gk := gvk.GroupKind()
		if v, exists := gvks[gk]; !exists || apiversion.MustCompare(v, gvk.Version) < 0 {
			gvks[gk] = gvk.Version
		}
	}

	items := make([]rscoreapi.GenericResourceService, 0)
	for gk, v := range gvks {
		if !selector.Matches(gk) {
			continue
		}

		mapping, err := mapper.RESTMapping(gk, v)
		if meta.IsNoMatchError(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		apiType := kmapi.NewResourceID(mapping)

		attrs := authorizer.AttributesRecord{
			User:            user,
			Verb:            "get",
			Namespace:       ns,
			APIGroup:        mapping.Resource.Group,
			Resource:        mapping.Resource.Resource,
			Name:            "",
			ResourceRequest: true,
		}

		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gk.Group,
			Version: v,
			Kind:    gk.Kind,
		})
		if err := r.kc.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			attrs.Name = item.GetName()
			attrs.Namespace = item.GetNamespace()
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

	result := rscoreapi.GenericResourceServiceList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    items,
	}

	return &result, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func (r *Storage) toGenericResourceService(item unstructured.Unstructured, apiType *kmapi.ResourceID, cmeta *kmapi.ClusterMetadata) (*rscoreapi.GenericResourceService, error) {
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

	genres := rscoreapi.GenericResourceService{
		// TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:         rscoreapi.GetGenericResourceName(&item),
			GenerateName: item.GetGenerateName(),
			Namespace:    item.GetNamespace(),
			// SelfLink:                   "",
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
			// ZZZ_DeprecatedClusterName: item.GetZZZ_DeprecatedClusterName(),
			// ManagedFields:              nil,
		},
		Spec: rscoreapi.GenericResourceServiceSpec{
			Cluster: *cmeta,
			APIType: *apiType,
			Name:    item.GetName(),
			Status: rscoreapi.GenericResourceServiceStatus{
				Status:  s.Status.String(),
				Message: s.Message,
			},
			Facilities: rscoreapi.GenericResourceServiceFacilities{
				Exposed: rscoreapi.GenericResourceServiceFacilitator{
					Usage: rscoreapi.FacilityUnknown,
				},
				TLS: rscoreapi.GenericResourceServiceFacilitator{
					Usage: rscoreapi.FacilityUnknown,
				},
				Backup: rscoreapi.GenericResourceServiceFacilitator{
					Usage: rscoreapi.FacilityUnknown,
				},
				Monitoring: rscoreapi.GenericResourceServiceFacilitator{
					Usage: rscoreapi.FacilityUnknown,
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
				ByLabel: kmapi.EdgeLabelExposedBy,
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
			genres.Spec.Facilities.Exposed.Usage = rscoreapi.FacilityUsed
			genres.Spec.Facilities.Exposed.Resource = rid
			genres.Spec.Facilities.Exposed.Refs = refs
		} else {
			genres.Spec.Facilities.Exposed.Usage = rscoreapi.FacilityUnused
		}
	}
	if apiType.Group == "kubedb.com" {
		rid, objs, err := graph.ExecQuery(r.kc, oid, sharedapi.ResourceLocator{
			Ref: metav1.GroupKind{
				Group: "catalog.appscode.com",
				Kind:  apiType.Kind + "Binding",
			},
			Query: sharedapi.ResourceQuery{
				Type:    sharedapi.GraphQLQuery,
				ByLabel: kmapi.EdgeLabelExposedBy,
			},
		})
		if err == nil {
			var gw *ofst.Gateway
			for _, obj := range objs {
				if gw == nil || obj.GetNamespace() == item.GetNamespace() {
					var binding catalogapi.GenericBinding
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &binding); err != nil {
						return nil, err
					}

					if binding.Status.Gateway != nil {
						// prefer root binding
						if obj.GetNamespace() == item.GetNamespace() {
							gw = binding.Status.Gateway
							break
						}
						// otherwise keep the first not nil gateway
						gw = binding.Status.Gateway
					}
				}
			}
			genres.Spec.Facilities.Gateway = gw
			if gw != nil &&
				(gw.Hostname != "" || gw.IP != "") &&
				genres.Spec.Facilities.Exposed.Usage == rscoreapi.FacilityUnused {

				genres.Spec.Facilities.Exposed.Usage = rscoreapi.FacilityUsed
				genres.Spec.Facilities.Exposed.Resource = rid
				genres.Spec.Facilities.Exposed.Refs = []kmapi.ObjectReference{
					{
						Namespace: gw.Namespace,
						Name:      gw.Name,
					},
				}
			}
		} else if !meta.IsNoMatchError(err) {
			return nil, err
		}
	}
	{
		yes, err := resourcemetrics.UsesTLS(content)
		if err != nil && !errors.Is(err, api.ErrMissingRefObject) {
			return nil, err
		}
		if yes {
			genres.Spec.Facilities.TLS.Usage = rscoreapi.FacilityUsed
		} else {
			genres.Spec.Facilities.TLS.Usage = rscoreapi.FacilityUnused
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
		if len(refs) > 0 {
			genres.Spec.Facilities.Backup.Usage = rscoreapi.FacilityUsed
			genres.Spec.Facilities.Backup.Resource = rid
			genres.Spec.Facilities.Backup.Refs = refs
		} else if err != nil && !meta.IsNoMatchError(err) {
			return nil, err
		}
	}
	if genres.Spec.Facilities.Backup.Usage == rscoreapi.FacilityUnknown {
		rid, refs, err := graph.ExecRawQuery(r.kc, oid, sharedapi.ResourceLocator{
			Ref: metav1.GroupKind{
				Group: "core.kubestash.com",
				Kind:  "BackupConfiguration",
			},
			Query: sharedapi.ResourceQuery{
				Type:    sharedapi.GraphQLQuery,
				ByLabel: kmapi.EdgeLabelBackupVia,
			},
		})
		if len(refs) > 0 {
			genres.Spec.Facilities.Backup.Usage = rscoreapi.FacilityUsed
			genres.Spec.Facilities.Backup.Resource = rid
			genres.Spec.Facilities.Backup.Refs = refs
		} else if err == nil {
			genres.Spec.Facilities.Backup.Usage = rscoreapi.FacilityUnused
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
		if len(refs) > 0 {
			genres.Spec.Facilities.Monitoring.Usage = rscoreapi.FacilityUsed
			genres.Spec.Facilities.Monitoring.Resource = rid
			genres.Spec.Facilities.Monitoring.Refs = refs
		} else if err != nil && !meta.IsNoMatchError(err) {
			return nil, err
		}

		if genres.Spec.Facilities.Monitoring.Usage == rscoreapi.FacilityUnknown {
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
			if len(refs) > 0 {
				genres.Spec.Facilities.Monitoring.Usage = rscoreapi.FacilityUsed
				genres.Spec.Facilities.Monitoring.Resource = rid
				genres.Spec.Facilities.Monitoring.Refs = refs
			} else if err == nil {
				genres.Spec.Facilities.Monitoring.Usage = rscoreapi.FacilityUnused
			} else if !meta.IsNoMatchError(err) {
				return nil, err
			}
		}
	}
	{
		buf := shared.BufferPool.Get().(*bytes.Buffer)
		defer shared.BufferPool.Put(buf)

		gvr := apiType.GroupVersionResource()
		if rd, err := resourcedescriptors.LoadByGVR(gvr); err == nil {
			execServices := make([]rscoreapi.ExecServiceFacilitator, 0, len(rd.Spec.Exec))

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

				if shared.IsPod(gvr) {
					execServices = append(execServices, rscoreapi.ExecServiceFacilitator{
						Alias:    exec.Alias,
						Resource: "pods",
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

					execServices = append(execServices, rscoreapi.ExecServiceFacilitator{
						Alias:    exec.Alias,
						Resource: "services",
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
