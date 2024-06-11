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

package clustermetadata

import (
	"context"

	"kubeops.dev/ui-server/pkg/b3"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
	clustermeta "kmodules.xyz/client-go/cluster"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ClusterMetadataReconciler reconciles a ClusterMetadata object
type ClusterMetadataReconciler struct {
	kc        client.Client
	bc        *b3.Client
	clusterID string
}

var _ reconcile.Reconciler = &ClusterMetadataReconciler{}

func NewReconciler(kc client.Client, bc *b3.Client) *ClusterMetadataReconciler {
	return &ClusterMetadataReconciler{
		kc: kc,
		bc: bc,
	}
}

func (r *ClusterMetadataReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	md, err := r.bc.Identify(r.clusterID)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = clustermeta.UpsertClusterMetadata(r.kc, md)
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterMetadataReconciler) SetupWithManager(mgr ctrl.Manager) error {
	filter := func(object client.Object) bool {
		return object.GetName() == kmapi.AceInfoConfigMapName &&
			object.GetNamespace() == metav1.NamespacePublic
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&core.ConfigMap{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return filter(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectOld == nil {
					return false
				}
				if e.ObjectNew == nil {
					return false
				}
				if e.ObjectNew.GetResourceVersion() == e.ObjectOld.GetResourceVersion() {
					return false
				}
				return filter(e.ObjectNew)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return filter(e.Object)
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return filter(e.Object)
			},
		})).
		Complete(r)
}
