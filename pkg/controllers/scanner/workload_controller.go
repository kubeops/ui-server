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

package scanner

import (
	"context"

	api "kubeops.dev/scanner/apis/reports/v1alpha1"
	scannerapi "kubeops.dev/scanner/apis/scanner/v1alpha1"
	"kubeops.dev/ui-server/pkg/shared"

	"github.com/pkg/errors"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/client-go/client/apiutil"
	"kmodules.xyz/client-go/client/duck"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// WorkloadReconciler reconciles a Workload object
type WorkloadReconciler struct {
	client.Client
}

var _ duck.Reconciler = &WorkloadReconciler{}

func (r *WorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var wl api.Workload
	if err := r.Get(ctx, req.NamespacedName, &wl); err != nil {
		log.Error(err, "unable to fetch Workload")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	sel, err := metav1.LabelSelectorAsSelector(wl.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, err
	}

	// list pods with selector
	var pods unstructured.UnstructuredList
	pods.SetAPIVersion("v1")
	pods.SetKind("Pod")
	err = r.List(context.TODO(), &pods,
		client.InNamespace(wl.Namespace),
		client.MatchingLabelsSelector{Selector: sel})
	if err != nil {
		return ctrl.Result{}, err
	}

	// get all the images into map
	refs := map[string]kmapi.PullCredentials{}
	for _, p := range pods.Items {
		var pod core.Pod
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(p.UnstructuredContent(), &pod); err != nil {
			return ctrl.Result{}, err
		}
		if pod.Status.Phase != core.PodRunning && pod.Status.Phase != core.PodSucceeded && pod.Status.Phase != "Completed" {
			return ctrl.Result{}, nil
		}
		refs, err = apiutil.CollectPullCredentials(&pod, refs)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	for ref, info := range refs {
		if should, err := r.shouldScan(ref); err != nil {
			return ctrl.Result{}, err // some serious error occurred
		} else if should {
			err := shared.SendScanRequest(ctx, r.Client, ref, info)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to send scan request for image=%s", ref)
			}
		} else {
			log.V(5).Info("skipped sending scan request", "image", ref)
		}
	}

	return ctrl.Result{}, nil
}

func (r *WorkloadReconciler) shouldScan(ref string) (bool, error) {
	var rep scannerapi.ImageScanReport
	err := r.Get(context.TODO(), types.NamespacedName{
		Name: scannerapi.GetReportName(ref),
	}, &rep)
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	return rep.Status.Phase == scannerapi.ImageScanReportPhaseOutdated, nil
}

func (r *WorkloadReconciler) InjectClient(c client.Client) error {
	r.Client = c
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return duck.ControllerManagedBy(mgr).
		For(&api.Workload{}).
		WithUnderlyingTypes(
			// use Unstructured obj, since it is already cached
			ObjectOf(core.SchemeGroupVersion.WithKind("ReplicationController")),
			ObjectOf(apps.SchemeGroupVersion.WithKind("Deployment")),
			ObjectOf(apps.SchemeGroupVersion.WithKind("StatefulSet")),
			ObjectOf(apps.SchemeGroupVersion.WithKind("DaemonSet")),
			ObjectOf(batch.SchemeGroupVersion.WithKind("Job")),
			ObjectOf(batch.SchemeGroupVersion.WithKind("CronJob")),
		).
		Complete(func() duck.Reconciler {
			return new(WorkloadReconciler)
		})
}

func ObjectOf(gvk schema.GroupVersionKind) client.Object {
	var u unstructured.Unstructured
	u.GetObjectKind().SetGroupVersionKind(gvk)
	return &u
}
