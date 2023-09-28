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

package projectquota

import (
	"context"
	"fmt"
	"sort"
	"strings"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"kmodules.xyz/apiversion"
	kutil "kmodules.xyz/client-go"
	cu "kmodules.xyz/client-go/client"
	clustermeta "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/resource-metadata/apis/management/v1alpha1"
	resourcemetrics "kmodules.xyz/resource-metrics"
	"kmodules.xyz/resource-metrics/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ProjectQuotaReconciler reconciles a ProjectQuota object
type ProjectQuotaReconciler struct {
	client.Client
	Discovery discovery.DiscoveryInterface
	Scheme    *runtime.Scheme
}

func (r *ProjectQuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var pj v1alpha1.ProjectQuota
	if err := r.Get(ctx, req.NamespacedName, &pj); err != nil {
		log.Error(err, "unable to fetch ProjectQuota")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var err error
	var vt kutil.VerbType
	vt, err = cu.CreateOrPatch(context.TODO(), r.Client, &pj, func(in client.Object, createOp bool) client.Object {
		obj := in.(*v1alpha1.ProjectQuota)
		err = r.CalculateStatus(&pj)
		return obj
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	log.Info(string(vt) + " ProjectQuota")

	return ctrl.Result{}, nil
}

type APIType struct {
	Group      string
	Kind       string
	Resource   string
	Versions   []string
	Namespaced bool
}

// handle non-namespaced resource limits
func (r *ProjectQuotaReconciler) ListKinds() (map[string]APIType, error) {
	_, resourceList, err := r.Discovery.ServerGroupsAndResources()

	apiTypes := map[string]APIType{}
	if discovery.IsGroupDiscoveryFailedError(err) || err == nil {
		for _, resources := range resourceList {
			gv, err := schema.ParseGroupVersion(resources.GroupVersion)
			if err != nil {
				return nil, err
			}
			for _, resource := range resources.APIResources {
				if strings.ContainsRune(resource.Name, '/') {
					continue
				}

				gk := schema.GroupKind{
					Group: gv.Group,
					Kind:  resource.Kind,
				}
				x, found := apiTypes[gk.String()]
				if !found {
					x = APIType{
						Group:      gv.Group,
						Kind:       resource.Kind,
						Resource:   resource.Name,
						Versions:   []string{gv.Version},
						Namespaced: resource.Namespaced,
					}
				} else {
					x.Versions = append(x.Versions, gv.Version)
				}
				apiTypes[gk.String()] = x
			}
		}
	}

	for gk, x := range apiTypes {
		if len(x.Versions) > 1 {
			sort.Slice(x.Versions, func(i, j int) bool {
				return apiversion.MustCompare(x.Versions[i], x.Versions[j]) > 0
			})
			apiTypes[gk] = x
		}
	}

	return apiTypes, nil
}

func (r *ProjectQuotaReconciler) CalculateStatus(pj *v1alpha1.ProjectQuota) error {
	var nsList core.NamespaceList
	err := r.List(context.TODO(), &nsList, client.MatchingLabels{
		clustermeta.LabelKeyRancherFieldProjectId: pj.Name,
	})
	if err != nil {
		return err
	}

	// init status
	pj.Status.Quotas = make([]v1alpha1.ResourceQuotaStatus, len(pj.Spec.Quotas))
	for i := range pj.Spec.Quotas {
		pj.Status.Quotas[i] = v1alpha1.ResourceQuotaStatus{
			ResourceQuotaSpec: pj.Spec.Quotas[i],
			Used:              core.ResourceList{},
		}
	}

	apiTypes, err := r.ListKinds()
	if err != nil {
		return err
	}

	for _, ns := range nsList.Items {
		nsUsed := map[schema.GroupKind]core.ResourceList{}

		for i, quota := range pj.Status.Quotas {
			gk := schema.GroupKind{
				Group: quota.Group,
				Kind:  quota.Kind,
			}
			used, found := nsUsed[gk]
			if found {
				quota.Used = used
				pj.Status.Quotas[i] = quota
			} else if quota.Kind == "" {
				for _, typeInfo := range apiTypes {
					if typeInfo.Group == quota.Group {
						used, found := nsUsed[schema.GroupKind{
							Group: typeInfo.Group,
							Kind:  typeInfo.Kind,
						}]
						if !found {
							used, err = r.UsedQuota(ns.Name, typeInfo)
							if err != nil {
								return err
							}
							nsUsed[gk] = used
						}
						quota.Used = api.AddResourceList(quota.Used, used)
					}
				}
			} else {
				typeInfo, found := apiTypes[gk.String()]
				if !found {
					return fmt.Errorf("can't detect api type info for %+v", gk)
				}
				used, err := r.UsedQuota(ns.Name, typeInfo)
				if err != nil {
					return err
				}
				nsUsed[gk] = used
				quota.Used = api.AddResourceList(quota.Used, used)
			}

			pj.Status.Quotas[i] = quota
		}
	}

	return nil
}

func (r *ProjectQuotaReconciler) UsedQuota(ns string, typeInfo APIType) (core.ResourceList, error) {
	gk := schema.GroupKind{
		Group: typeInfo.Group,
		Kind:  typeInfo.Kind,
	}

	if !typeInfo.Namespaced {
		// Todo:
		// No opinion?
		return nil, fmt.Errorf("can't apply quota for non-namespaced resources %+v", gk)
	}

	var done bool
	var used core.ResourceList
	for _, version := range typeInfo.Versions {
		gvk := gk.WithVersion(version)
		if api.IsRegistered(gvk) {
			done = true

			var list unstructured.UnstructuredList
			list.SetGroupVersionKind(gvk)
			err := r.List(context.TODO(), &list, client.InNamespace(ns))
			if err != nil {
				return nil, err
			}

			for _, obj := range list.Items {
				content := obj.UnstructuredContent()

				usage := core.ResourceList{}

				// https://kubernetes.io/docs/concepts/policy/resource-quotas/#compute-resource-quota
				requests, err := resourcemetrics.AppResourceRequests(content)
				if err != nil {
					return nil, err
				}
				for k, v := range requests {
					usage["requests."+k] = v
				}
				limits, err := resourcemetrics.AppResourceLimits(content)
				if err != nil {
					return nil, err
				}
				for k, v := range limits {
					usage["limits."+k] = v
				}

				used = api.AddResourceList(used, usage)
			}
			break
		}
	}
	if !done {
		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(gk.WithVersion(typeInfo.Versions[0]))
		err := r.List(context.TODO(), &list, client.InNamespace(ns))
		if err != nil {
			return nil, err
		}
		if len(list.Items) > 0 {
			// Todo:
			// Don't error out
			return nil, fmt.Errorf("resource calculator not defined for %+v", gk)
		}
	}
	return used, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectQuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ProjectQuota{}).
		Complete(r)
}
