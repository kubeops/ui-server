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
	"time"

	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	log := logger.FromContext(ctx).WithValues("name", req.NamespacedName.Name)
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
		if result, err := finder.ListConnectedObjectIDs(&obj, rd.Spec.Connections); err != nil {
			// In case of discovery error, we don't return error because errors are rate limited.
			// We need to keep trying until the reconciliation is successful.
			if IsDiscoveryError(err) {
				log.Error(err, "unable to list connections", "group", r.R.Group, "kind", r.R.Kind)
				return reconcile.Result{RequeueAfter: 500 * time.Millisecond}, nil
			}
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return reconcile.Result{}, client.IgnoreNotFound(err)
		} else if gvk == gvkService && result[kmapi.EdgeLabelExposedBy].Len() == 0 {
			return reconcile.Result{RequeueAfter: 2 * time.Minute}, nil
		} else {
			objGraph.Update(kmapi.NewObjectID(&obj).OID(), result)
		}
	}

	return reconcile.Result{}, nil
}

func IsDiscoveryError(err error) bool {
	if errors.Is(err, &discovery.ErrGroupDiscoveryFailed{}) {
		return true
	}
	var errRDF *apiutil.ErrResourceDiscoveryFailed
	return errors.As(err, &errRDF)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	var obj unstructured.Unstructured
	obj.SetGroupVersionKind(r.R.GroupVersionKind())
	return builder.ControllerManagedBy(mgr).
		For(&obj).
		Complete(r)
}
