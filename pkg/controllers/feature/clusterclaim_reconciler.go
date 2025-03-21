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

package feature

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	kmapi "kmodules.xyz/client-go/api/v1"
	cu "kmodules.xyz/client-go/client"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

type ClusterClaimReconciler struct {
	kc client.Client
}

var _ reconcile.Reconciler = &ClusterClaimReconciler{}

func NewClusterClaimReconciler(kc client.Client) *ClusterClaimReconciler {
	return &ClusterClaimReconciler{
		kc: kc,
	}
}

func (r *ClusterClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	var featureList uiapi.FeatureList
	if err = r.kc.List(ctx, &featureList); err != nil {
		return ctrl.Result{}, err
	}

	var enabledFeatures, extFeatures, disabledFeatures []string
	for _, feature := range featureList.Items {
		if feature.Status.Enabled == nil {
			return ctrl.Result{}, fmt.Errorf("feature %s is not reconciled yet", feature.Name)
		}
		if ptr.Deref(feature.Status.Enabled, false) {
			enabledFeatures = append(enabledFeatures, feature.Name)
			if !ptr.Deref(feature.Status.Managed, false) {
				extFeatures = append(extFeatures, feature.Name)
			}
		}

		if feature.Spec.Disabled {
			disabledFeatures = append(disabledFeatures, feature.Name)
		}
	}

	sort.Strings(enabledFeatures)
	sort.Strings(extFeatures)
	sort.Strings(disabledFeatures)

	data, err := yaml.Marshal(kmapi.ClusterClaimFeatures{
		EnabledFeatures:           enabledFeatures,
		ExternallyManagedFeatures: extFeatures,
		DisabledFeatures:          disabledFeatures,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	obj := &clusterv1alpha1.ClusterClaim{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: kmapi.ClusterClaimKeyFeatures,
		},
	}
	_, err = cu.CreateOrPatch(context.TODO(), r.kc, obj, func(o client.Object, createOp bool) client.Object {
		in := o.(*clusterv1alpha1.ClusterClaim)
		in.Spec.Value = string(data)
		return in
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&uiapi.Feature{}).
		Named("ClusterClaimReconciler").
		Complete(r)
}
