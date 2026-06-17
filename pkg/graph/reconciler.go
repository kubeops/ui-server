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

package graph

import (
	"context"
	"errors"
	"slices"
	"time"

	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/discovery"
	kmapi "kmodules.xyz/client-go/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler reconciles a Release object
type Reconciler struct {
	client.Client
	R      kmapi.ResourceID
	Scheme *runtime.Scheme
}

var gvkService = core.SchemeGroupVersion.WithKind("Service")

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx).WithValues("name", req.Name)
	gvk := r.R.GroupVersionKind()

	var obj unstructured.Unstructured
	obj.SetGroupVersionKind(gvk)
	if err := r.Get(context.TODO(), req.NamespacedName, &obj); err != nil {
		if apierrors.IsNotFound(err) {
			oid := kmapi.ObjectID{
				Group:     gvk.Group,
				Kind:      gvk.Kind,
				Namespace: req.Namespace,
				Name:      req.Name,
			}
			objGraph.Delete(oid.OID())
		}

		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if rd, err := Registry.LoadByGVK(gvk); err == nil {
		finder := ObjectFinder{
			Client: r.Client,
		}
		// result holds the edges that resolved even when ferr != nil, so one bad
		// connection doesn't discard the rest of the object's graph.
		result, ferr := finder.ListConnectedObjectIDs(&obj, rd.Spec.Connections)

		// Discovery errors are transient: keep the existing graph and retry fast.
		if ferr != nil && anyDiscoveryError(ferr) {
			log.Error(ferr, "unable to list some connections", "group", r.R.Group, "kind", r.R.Kind)
			return reconcile.Result{RequeueAfter: 500 * time.Millisecond}, nil
		}

		if gvk == gvkService && result[kmapi.EdgeLabelExposedBy].Len() == 0 {
			return reconcile.Result{RequeueAfter: 2 * time.Minute}, nil
		}

		// Persist whatever resolved, even if some connections failed.
		objGraph.Update(kmapi.NewObjectID(&obj).OID(), result)

		if ferr != nil {
			// Partial graph already persisted; return err for exponential-backoff retry.
			log.Error(ferr, "some connections failed to resolve; graph updated with partial result", "group", r.R.Group, "kind", r.R.Kind)
			return reconcile.Result{}, client.IgnoreNotFound(ferr)
		}
	}

	return reconcile.Result{}, nil
}

func IsDiscoveryError(err error) bool {
	var errRDF *apiutil.ErrResourceDiscoveryFailed
	if errors.As(err, &errRDF) {
		return true
	}
	return errors.Is(err, &discovery.ErrGroupDiscoveryFailed{})
}

// anyDiscoveryError reports whether err (or any error in an aggregate) is a
// discovery error; needed because the k8s aggregate type implements Is but not As.
func anyDiscoveryError(err error) bool {
	if agg, ok := err.(utilerrors.Aggregate); ok {
		return slices.ContainsFunc(utilerrors.Flatten(agg).Errors(), IsDiscoveryError)
	}
	return IsDiscoveryError(err)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	var obj unstructured.Unstructured
	obj.SetGroupVersionKind(r.R.GroupVersionKind())
	return builder.ControllerManagedBy(mgr).
		For(&obj).
		Named("ui-server-" + obj.GroupVersionKind().String()).
		Complete(r)
}
