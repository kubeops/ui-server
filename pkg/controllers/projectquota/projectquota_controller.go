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
	"sync"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/klog/v2"
	"kmodules.xyz/apiversion"
	kmapi "kmodules.xyz/client-go/api/v1"
	cu "kmodules.xyz/client-go/client"
	clustermeta "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/resource-metadata/apis/management/v1alpha1"
	resourcemetrics "kmodules.xyz/resource-metrics"
	"kmodules.xyz/resource-metrics/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ProjectQuotaReconciler reconciles a ProjectQuota object
type ProjectQuotaReconciler struct {
	client.Client
	disco  discovery.DiscoveryInterface
	scheme *runtime.Scheme

	mu       sync.Mutex
	ctrl     controller.Controller
	cache    cache.Cache
	regTypes map[schema.GroupVersionKind]bool
}

func NewReconciler(kc client.Client, disco discovery.DiscoveryInterface) *ProjectQuotaReconciler {
	return &ProjectQuotaReconciler{
		Client:   kc,
		disco:    disco,
		scheme:   kc.Scheme(),
		regTypes: make(map[schema.GroupVersionKind]bool),
	}
}

func (r *ProjectQuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var pj v1alpha1.ProjectQuota
	if err := r.Get(ctx, req.NamespacedName, &pj); err != nil {
		log.Error(err, "unable to fetch ProjectQuota")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	err := r.CalculateStatus(&pj)
	if err != nil {
		return ctrl.Result{}, err
	}

	vt, err := cu.PatchStatus(context.TODO(), r.Client, &pj, func(in client.Object) client.Object {
		obj := in.(*v1alpha1.ProjectQuota)
		obj.Status = pj.Status

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
	_, resourceList, err := r.disco.ServerGroupsAndResources()

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

type typeStatus struct {
	QuotaResult QuotaResult
	Used        core.ResourceList
}

type QuotaResult struct {
	Result v1alpha1.QuotaResult
	Reason string
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
			Result:            v1alpha1.ResultSuccess,
			Used:              core.ResourceList{},
		}
	}

	apiTypes, err := r.ListKinds()
	if err != nil {
		return err
	}

	for _, ns := range nsList.Items {
		nsUsed := map[schema.GroupKind]typeStatus{}

		for i, quota := range pj.Status.Quotas {
			// If previously set quotaStatus as error we can skip that as we've already assigned the reason
			if quota.Result == v1alpha1.ResultError {
				continue
			}

			gk := schema.GroupKind{
				Group: quota.Group,
				Kind:  quota.Kind,
			}
			used, found := nsUsed[gk]
			if found {
				quota.Used = used.Used
				quota.Result = used.QuotaResult.Result
				quota.Reason = used.QuotaResult.Reason
				pj.Status.Quotas[i] = quota

			} else if quota.Kind == "" {
				isGroupFound := false

				for _, typeInfo := range apiTypes {
					if typeInfo.Group == quota.Group {
						isGroupFound = true

						used, found := nsUsed[schema.GroupKind{
							Group: typeInfo.Group,
							Kind:  typeInfo.Kind,
						}]
						if !found {
							q, qr, err := r.UsedQuota(ns.Name, typeInfo)
							if err != nil {
								return err
							}
							used = typeStatus{
								QuotaResult: *qr,
								Used:        q,
							}
							nsUsed[gk] = used
						}

						quota.Used = api.AddResourceList(quota.Used, used.Used)
						if used.QuotaResult.Result == v1alpha1.ResultError {
							quota.Result = used.QuotaResult.Result
							quota.Reason = used.QuotaResult.Reason
						}
					}
				}
				if !isGroupFound {
					quota.Result = v1alpha1.ResultError
					quota.Reason = "API Group doesn't exits"
				}
			} else {
				typeInfo, found := apiTypes[gk.String()]
				if !found {
					quota.Result = v1alpha1.ResultError
					quota.Reason = "Provided API Info is not valid"
				} else {
					used, qr, err := r.UsedQuota(ns.Name, typeInfo)
					if err != nil {
						return err
					}
					nsUsed[gk] = typeStatus{
						QuotaResult: *qr,
						Used:        used,
					}
					quota.Used = api.AddResourceList(quota.Used, used)
				}
			}

			pj.Status.Quotas[i] = quota
		}
	}

	return nil
}

func (r *ProjectQuotaReconciler) UsedQuota(ns string, typeInfo APIType) (core.ResourceList, *QuotaResult, error) {
	gk := schema.GroupKind{
		Group: typeInfo.Group,
		Kind:  typeInfo.Kind,
	}

	// If found non-namespaced resource for a group we can invalidate that quota
	if !typeInfo.Namespaced {
		return nil, &QuotaResult{
			Result: v1alpha1.ResultError,
			Reason: fmt.Sprintf("Group `%s` has non-namespaced resource `%s`", typeInfo.Group, typeInfo.Resource),
		}, nil
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
				return nil, nil, err
			}

			for _, obj := range list.Items {
				content := obj.UnstructuredContent()

				usage := core.ResourceList{}

				// https://kubernetes.io/docs/concepts/policy/resource-quotas/#compute-resource-quota
				requests, err := resourcemetrics.AppResourceRequests(content)
				if err != nil {
					return nil, nil, err
				}
				for k, v := range requests {
					usage["requests."+k] = v
				}
				limits, err := resourcemetrics.AppResourceLimits(content)
				if err != nil {
					return nil, nil, err
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
			return nil, nil, err
		}
		if len(list.Items) > 0 {
			return nil, &QuotaResult{
				Result: v1alpha1.ResultError,
				Reason: "Resource calculator not defined",
			}, nil
		}
	}
	return used, &QuotaResult{Result: v1alpha1.ResultSuccess}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectQuotaReconciler) SetupWithManager(mgr ctrl.Manager) (*ProjectQuotaReconciler, error) {
	ctrl, err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ProjectQuota{}).
		Build(r)
	if err != nil {
		return nil, err
	}
	r.ctrl = ctrl
	r.cache = mgr.GetCache()
	return r, nil
}

func (r *ProjectQuotaReconciler) StartWatcher(rid kmapi.ResourceID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ctrl == nil {
		klog.Fatalln("ProjectQuota reconciler is not setup yet!")
	}

	gvk := rid.GroupVersionKind()
	if gvk.Kind == "" {
		klog.Fatalln("can't start ProjectQuota reconciler for unknown Kind!")
	}

	if api.IsRegistered(gvk) && !r.regTypes[gvk] {
		var obj unstructured.Unstructured
		obj.SetGroupVersionKind(gvk)
		err := r.ctrl.Watch(
			source.Kind(r.cache, &obj),
			handler.EnqueueRequestsFromMapFunc(ProjectQuotaForObjects(r.Client)),
		)
		if err != nil {
			klog.Fatalln(err)
		}
		r.regTypes[gvk] = true
	}
}

// Obj -> ProjectQuota
func ProjectQuotaForObjects(kc client.Client) func(_ context.Context, _ client.Object) []reconcile.Request {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		if obj.GetNamespace() == "" {
			return nil
		}

		var ns core.Namespace
		err := kc.Get(ctx, client.ObjectKey{Name: obj.GetNamespace()}, &ns)
		if err != nil {
			klog.Error(err)
			return nil
		}

		projectId, found := ns.Labels[clustermeta.LabelKeyRancherFieldProjectId]
		if !found {
			return nil
		}
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{Name: projectId}},
		}
	}
}
