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
	"hash/fnv"
	"strconv"

	api "kubeops.dev/scanner/apis/reports/v1alpha1"
	scannerapi "kubeops.dev/scanner/apis/scanner/v1alpha1"

	cache "github.com/go-pkgz/expirable-cache/v2"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/client-go/client/apiutil"
	"kmodules.xyz/client-go/client/duck"
	"kmodules.xyz/client-go/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// WorkloadReconciler reconciles a Workload object
type WorkloadReconciler struct {
	client.Client
	Cache cache.Cache[string, string]
}

var _ duck.Reconciler = &WorkloadReconciler{}

func (r *WorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var mypod api.Workload
	if err := r.Get(ctx, req.NamespacedName, &mypod); err != nil {
		log.Error(err, "unable to fetch Workload")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	refs := map[string]kmapi.PullSecrets{}

	if mypod.Kind == "Job" || mypod.Kind == "CronJob" {
		for _, c := range mypod.Spec.Template.Spec.Containers {
			refs[c.Image] = kmapi.PullSecrets{
				Namespace: mypod.Namespace,
				Refs:      mypod.Spec.Template.Spec.ImagePullSecrets,
			}
		}
		for _, c := range mypod.Spec.Template.Spec.InitContainers {
			refs[c.Image] = kmapi.PullSecrets{
				Namespace: mypod.Namespace,
				Refs:      mypod.Spec.Template.Spec.ImagePullSecrets,
			}
		}
		for _, c := range mypod.Spec.Template.Spec.EphemeralContainers {
			refs[c.Image] = kmapi.PullSecrets{
				Namespace: mypod.Namespace,
				Refs:      mypod.Spec.Template.Spec.ImagePullSecrets,
			}
		}
	} else {
		sel, err := metav1.LabelSelectorAsSelector(mypod.Spec.Selector)
		if err != nil {
			return ctrl.Result{}, err
		}

		var pods unstructured.UnstructuredList
		pods.SetAPIVersion("v1")
		pods.SetKind("Pod")
		err = r.List(context.TODO(), &pods,
			client.InNamespace(mypod.Namespace),
			client.MatchingLabelsSelector{Selector: sel})
		if err != nil {
			return ctrl.Result{}, err
		}

		for _, p := range pods.Items {
			var pod core.Pod
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(p.UnstructuredContent(), &pod); err != nil {
				return ctrl.Result{}, err
			}
			refs, err = apiutil.CollectPullSecrets(&pod, refs)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	for ref, info := range refs {
		if hash, ok := r.shouldScan(ref, info); ok {
			obj := scannerapi.ImageScanRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: GenerateName(ref),
				},
				Spec: scannerapi.ImageScanRequestSpec{
					ImageRef:    ref,
					Namespace:   info.Namespace,
					PullSecrets: info.Refs,
				},
			}
			if err := r.Client.Create(ctx, &obj); err != nil {
				if r.Cache != nil {
					r.Cache.Set(ref, hash, 0)
				}
				return ctrl.Result{}, err
			}
		} else {
			log.V(5).Info("skipped sending scan request", "image", ref)
		}
	}

	return ctrl.Result{}, nil
}

func (r *WorkloadReconciler) shouldScan(ref string, info kmapi.PullSecrets) (string, bool) {
	if r.Cache == nil {
		return "", true
	}

	h := fnv.New64a()
	meta.DeepHashObject(h, info)
	newHash := strconv.FormatUint(h.Sum64(), 10)

	curHash, found := r.Cache.Get(ref)
	if !found {
		return newHash, true
	}
	return newHash, curHash != newHash
}

func GenerateName(s string) string {
	const max = 64 - 8
	if len(s) < max {
		return s + "-"
	}
	return s[max:] + "-"
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
