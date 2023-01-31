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

package controllers

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	fluxcd "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/go-logr/logr"
	"gomodules.xyz/pointer"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kmc "kmodules.xyz/client-go/client"
	meta_util "kmodules.xyz/client-go/meta"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// FeatureReconciler reconciles a Feature object
type FeatureReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	APIReader client.Reader
}

const (
	UIServerCleanupFinalizer       = "ui-server.kubeops.dev/cleanup"
	ManagerAppsCodeContainerEngine = "ACE"
	featureSetReferencePath        = ".spec.featureSet"
)

type frReconciler struct {
	client    client.Client
	apiReader client.Reader
	logger    logr.Logger
	feature   *uiapi.Feature
	releases  *fluxcd.HelmReleaseList
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *FeatureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling......")

	f := &uiapi.Feature{}
	err := r.Get(ctx, req.NamespacedName, f)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	fr := frReconciler{
		client:    r.Client,
		apiReader: r.APIReader,
		logger:    logger,
		feature:   f,
	}

	if fr.feature.DeletionTimestamp != nil {
		err := fr.updateFeatureSetAndRemoveFinalizer(ctx)
		if err != nil && kerr.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	err = fr.reconcile(ctx)
	if err != nil && !kerr.IsNotFound(err) {
		logger.Error(err, "failed to reconcile")
		return ctrl.Result{}, nil
	}

	if err := fr.updateStatus(ctx); err != nil {
		return ctrl.Result{}, err
	}

	err = fr.updateFeatureSetEntry(ctx)
	if err != nil && !kerr.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// We need to requeue the Feature periodically so that it works with third party resources.
	// Here, we are requeueing every Feature after a random interval between 30s to 150s so that
	// all Features do not get requeued at the same time.
	requeueInterval := getRandomInterval()
	fr.logger.Info(fmt.Sprintf("Requeuing after %v", requeueInterval.String()))
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *frReconciler) reconcile(ctx context.Context) error {
	if err := r.ensureFinalizer(ctx); err != nil {
		return err
	}
	r.resetFeatureStatus()

	enabled, reason, err := r.isFeatureEnabled(ctx)
	if err != nil {
		r.feature.Status.Enabled = pointer.BoolP(false)
		r.feature.Status.Note = err.Error()
		r.logger.Error(err, "Failed to check whether Feature is enabled or not")
		return nil
	}
	if !enabled {
		r.feature.Status.Enabled = pointer.BoolP(false)
		r.feature.Status.Note = reason
		return nil
	}
	r.feature.Status.Enabled = pointer.BoolP(true)

	managed, reason, err := r.isFeatureManaged(ctx)
	if err != nil {
		r.feature.Status.Managed = pointer.BoolP(false)
		r.feature.Status.Note = err.Error()
		r.logger.Error(err, "Failed to check whether Feature is managed or not")
		return nil
	}
	if !managed {
		r.feature.Status.Managed = pointer.BoolP(false)
		r.feature.Status.Note = reason
		return nil
	}
	r.feature.Status.Managed = pointer.BoolP(true)

	ready, reason := r.isFeatureReady()
	if !ready {
		r.feature.Status.Ready = pointer.BoolP(false)
		r.feature.Status.Note = reason
		return nil
	}
	r.feature.Status.Ready = pointer.BoolP(true)

	return nil
}

func (r *frReconciler) ensureFinalizer(ctx context.Context) error {
	if changed := controllerutil.AddFinalizer(r.feature, UIServerCleanupFinalizer); changed {
		_, _, err := kmc.CreateOrPatch(ctx, r.client, r.feature.DeepCopy(), func(obj client.Object, createOp bool) client.Object {
			in := obj.(*uiapi.Feature)
			in.ObjectMeta.Finalizers = r.feature.Finalizers
			return in
		})
		return err
	}
	return nil
}

func (r *frReconciler) resetFeatureStatus() {
	r.feature.Status = uiapi.FeatureStatus{}
}

func (r *frReconciler) isFeatureEnabled(ctx context.Context) (bool, string, error) {
	exist, reason, err := r.checkDependencyExistence(ctx)
	if err != nil {
		return false, "", err
	}
	if !exist {
		return false, reason, nil
	}

	exist, reason, err = r.checkRequiredResourcesExistence(ctx)
	if err != nil {
		return false, "", err
	}
	if !exist {
		return false, reason, err
	}

	exist, reason, err = r.checkRequiredWorkloadExistence(ctx)
	if err != nil {
		return false, "", err
	}
	if !exist {
		return false, reason, err
	}

	return true, "", nil
}

func (r *frReconciler) isFeatureManaged(ctx context.Context) (bool, string, error) {
	releases, err := r.findManagedHelmReleases(ctx)
	if err != nil {
		return false, "", err
	}

	if len(releases.Items) == 0 {
		return false, "Respective HelmRelease does not exist", nil
	}
	r.releases = releases
	return true, "", nil
}

func (r *frReconciler) isFeatureReady() (bool, string) {
	if r.releases == nil || len(r.releases.Items) == 0 {
		return false, "Feature is not managed by the UI"
	}
	if !areReleasesReady(r.releases.Items) {
		return false, "Feature is not ready yet."
	}
	return true, ""
}

func (r *frReconciler) checkDependencyExistence(ctx context.Context) (bool, string, error) {
	for _, d := range r.feature.Spec.Requirements.Features {
		f := &uiapi.Feature{}
		if err := r.client.Get(ctx, types.NamespacedName{Name: d}, f); err != nil {
			if kerr.IsNotFound(err) {
				return false, fmt.Sprintf("Dependency not satisfied. Feature %q does not exist.", d), nil
			}
			return false, "", err
		}

		if f.Status.Enabled != nil && !*f.Status.Enabled {
			return false, fmt.Sprintf("Dependency not satisfied. Feature %q is not enabled.", d), nil
		}
	}
	return true, "", nil
}

func (r *frReconciler) checkRequiredResourcesExistence(ctx context.Context) (bool, string, error) {
	for _, gvk := range r.feature.Spec.Requirements.Resources {
		objList := unstructured.UnstructuredList{}
		objList.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind,
		})
		if err := r.apiReader.List(ctx, &objList, &client.ListOptions{Limit: 1}); err != nil {
			if meta.IsNoMatchError(err) {
				return false, fmt.Sprintf("Required resource %q is not registered.", gvk.String()), err
			}
			return false, "", err
		}
	}
	return true, "", nil
}

func (r *frReconciler) checkRequiredWorkloadExistence(ctx context.Context) (bool, string, error) {
	for _, w := range r.feature.Spec.Requirements.Workloads {
		objList := unstructured.UnstructuredList{}
		objList.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   w.Group,
			Version: w.Version,
			Kind:    w.Kind,
		})
		selector := labels.SelectorFromSet(w.Selector)
		if err := r.apiReader.List(ctx, &objList, &client.ListOptions{Limit: 1, LabelSelector: selector}); err != nil {
			if meta.IsNoMatchError(err) {
				return false, fmt.Sprintf("Required resource %q is not registered.", w.String()), err
			}
			return false, "", err
		}
		if len(objList.Items) == 0 {
			return false, "Required workload does not exist", nil
		}
	}
	return true, "", nil
}

func (r *frReconciler) findManagedHelmReleases(ctx context.Context) (*fluxcd.HelmReleaseList, error) {
	selector := labels.SelectorFromSet(map[string]string{
		meta_util.ComponentLabelKey: r.feature.Name,
		meta_util.PartOfLabelKey:    r.feature.Spec.FeatureSet,
	})

	releases := &fluxcd.HelmReleaseList{}
	err := r.client.List(ctx, releases, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	return releases, nil
}

func areReleasesReady(releases []fluxcd.HelmRelease) bool {
	for _, release := range releases {
		if !isReleaseReady(release.Status.Conditions) {
			return false
		}
	}
	return true
}

func isReleaseReady(conditions []metav1.Condition) bool {
	for i := range conditions {
		if conditions[i].Type == "Ready" && conditions[i].Status == "True" {
			return true
		}
	}
	return false
}

func (r *frReconciler) updateFeatureSetAndRemoveFinalizer(ctx context.Context) error {
	if err := r.updateFeatureSetEntry(ctx); err != nil {
		if !kerr.IsNotFound(err) {
			return err
		}
	}

	if changed := controllerutil.RemoveFinalizer(r.feature, UIServerCleanupFinalizer); changed {
		_, _, err := kmc.CreateOrPatch(ctx, r.client, r.feature.DeepCopy(), func(obj client.Object, createOp bool) client.Object {
			in := obj.(*uiapi.Feature)
			in.ObjectMeta.Finalizers = r.feature.Finalizers
			return in
		})
		return err
	}
	return nil
}

func (r *frReconciler) updateFeatureSetEntry(ctx context.Context) error {
	fs := &uiapi.FeatureSet{}
	err := r.client.Get(ctx, types.NamespacedName{Name: r.feature.Spec.FeatureSet}, fs)
	if err != nil {
		return err
	}

	found := false
	for i, f := range fs.Status.Features {
		if f.Name == r.feature.Name {
			fs.Status.Features[i].Enabled = r.feature.Status.Enabled
			found = true
		}
	}
	if !found {
		fs.Status.Features = append(fs.Status.Features, uiapi.ComponentStatus{
			Name:    r.feature.Name,
			Enabled: r.feature.Status.Enabled,
		})
	}
	return r.updateFeatureSetStatus(ctx, fs)
}

func (r *frReconciler) updateFeatureSetStatus(ctx context.Context, fs *uiapi.FeatureSet) error {
	fs.Status.Enabled = pointer.BoolP(true)
	fs.Status.Note = ""
	enabled, reason := allRequireFeaturesEnabled(fs)
	if !enabled {
		fs.Status.Enabled = pointer.BoolP(false)
		fs.Status.Note = reason
	}

	if !atLeastOneFeatureEnabled(fs.Status.Features) {
		fs.Status.Enabled = pointer.BoolP(false)
		fs.Status.Note = "No feature enabled yet for this feature set."
	}
	_, _, err := kmc.PatchStatus(
		ctx,
		r.client,
		fs.DeepCopy(),
		func(obj client.Object) client.Object {
			in := obj.(*uiapi.FeatureSet)
			in.Status = fs.Status
			return in
		},
	)
	return client.IgnoreNotFound(err)
}

func allRequireFeaturesEnabled(fs *uiapi.FeatureSet) (enabled bool, reason string) {
	for _, f := range fs.Spec.RequiredFeatures {
		if !isEnabled(f, fs.Status.Features) {
			return false, fmt.Sprintf("Required feature '%s' is not enabled.", f)
		}
	}
	return true, ""
}

func isEnabled(featureName string, status []uiapi.ComponentStatus) bool {
	for i := range status {
		if status[i].Name == featureName && (status[i].Enabled != nil && *status[i].Enabled) {
			return true
		}
	}
	return false
}

func atLeastOneFeatureEnabled(status []uiapi.ComponentStatus) bool {
	for i := range status {
		if status[i].Enabled != nil && *status[i].Enabled {
			return true
		}
	}
	return false
}

func (r *frReconciler) updateStatus(ctx context.Context) error {
	_, _, err := kmc.PatchStatus(
		ctx,
		r.client,
		r.feature.DeepCopy(),
		func(obj client.Object) client.Object {
			in := obj.(*uiapi.Feature)
			in.Status = r.feature.Status
			return in
		},
	)
	return client.IgnoreNotFound(err)
}

// SetupWithManager sets up the controller with the Manager.
func (r *FeatureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &uiapi.Feature{}, featureSetReferencePath, func(object client.Object) []string {
		feature := object.(*uiapi.Feature)
		return []string{feature.Spec.FeatureSet}
	})
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&uiapi.Feature{}).
		Watches(
			&source.Kind{Type: &uiapi.FeatureSet{}},
			handler.EnqueueRequestsFromMapFunc(r.findFeaturesForFeatureSet),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(
			&source.Kind{Type: &fluxcd.HelmRelease{}},
			handler.EnqueueRequestsFromMapFunc(r.findFeatureForHelmRelease),
		).
		Complete(r)
}

func (r *FeatureReconciler) findFeaturesForFeatureSet(featureSet client.Object) []reconcile.Request {
	featureList := &uiapi.FeatureList{}
	err := r.List(context.Background(), featureList, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(featureSetReferencePath, featureSet.GetName()),
	})
	if err != nil {
		return []reconcile.Request{}
	}
	requests := make([]reconcile.Request, len(featureList.Items))
	for i := range featureList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: featureList.Items[i].Name,
			},
		}
	}
	return requests
}

func (r *FeatureReconciler) findFeatureForHelmRelease(release client.Object) []reconcile.Request {
	manager, err := meta_util.GetStringValueForKeys(release.GetLabels(), meta_util.ManagedByLabelKey)
	if err != nil || manager != ManagerAppsCodeContainerEngine {
		return []reconcile.Request{}
	}
	featureName, err := meta_util.GetStringValueForKeys(release.GetLabels(), meta_util.ComponentLabelKey)
	if err != nil {
		return []reconcile.Request{}
	}
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name: featureName,
			},
		},
	}
}

func getRandomInterval() time.Duration {
	minSecond := 30
	maxSecond := 120
	offset := rand.Int() % maxSecond
	return time.Second * time.Duration(minSecond+offset)
}
