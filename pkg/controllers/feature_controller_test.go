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
	"testing"

	fluxcd "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"gomodules.xyz/pointer"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"kmodules.xyz/client-go/meta"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFeatureShouldBeDisabledIfRequirementsNotSatisfied(t *testing.T) {
	testCases := map[string]struct {
		feature *uiapi.Feature
	}{
		"Dependency not satisfied": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.Requirements.Features = []string{"foo", "bar"}
			}),
		},
		"Required resources does not exist": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.Requirements.Resources = []metav1.GroupVersionKind{
					{
						Group:   "foo.io",
						Version: "v1",
						Kind:    "Foo",
					},
				}
			}),
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			r := frReconciler{
				client:  getFakeClient(t, tt.feature),
				logger:  dummyLogger(),
				feature: tt.feature,
			}
			err := r.reconcile(context.Background())
			assert.Nil(t, err)
			assert.NotEmpty(t, r.feature.Status.Note)
		})
	}
}

func TestManagedFieldIsSetProperly(t *testing.T) {
	feature := sampleFeature()

	testCases := map[string]struct {
		labels   map[string]string
		expected bool
	}{
		"Should be false when not managed by UI": {
			labels: map[string]string{
				meta.ManagedByLabelKey: "foo.io",
				meta.ComponentLabelKey: "bar",
				meta.PartOfLabelKey:    "baz",
			},
			expected: false,
		},
		"Should be true when managed by UI": {
			labels: map[string]string{
				meta.ManagedByLabelKey: "kubeops.dev",
				meta.ComponentLabelKey: feature.Name,
				meta.PartOfLabelKey:    feature.Spec.FeatureSet,
			},
			expected: true,
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			hr := sampleHelmRelease(func(in *fluxcd.HelmRelease) {
				in.Labels = tt.labels
			})
			r := frReconciler{
				client:  getFakeClient(t, feature.DeepCopy(), hr),
				logger:  dummyLogger(),
				feature: feature.DeepCopy(),
			}
			err := r.reconcile(context.Background())
			assert.Nil(t, err)
			if assert.NotNil(t, r.feature.Status.Managed) {
				assert.Equal(t, tt.expected, *r.feature.Status.Managed)
			}
		})
	}
}

func TestReadyFieldIsSetProperly(t *testing.T) {
	feature := sampleFeature()

	testCases := map[string]struct {
		conditionStatus metav1.ConditionStatus
		ready           bool
	}{
		"Should be false when HelmRelease is not Ready": {
			conditionStatus: metav1.ConditionFalse,
			ready:           false,
		},
		"Should be true when HelmRelease is Ready": {
			conditionStatus: metav1.ConditionTrue,
			ready:           true,
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			hr := sampleHelmRelease(func(in *fluxcd.HelmRelease) {
				in.Labels = map[string]string{
					meta.ManagedByLabelKey: "kubeops.dev",
					meta.ComponentLabelKey: feature.Name,
					meta.PartOfLabelKey:    feature.Spec.FeatureSet,
				}
				in.Status.Conditions = []metav1.Condition{
					{
						Type:   "Ready",
						Status: tt.conditionStatus,
					},
				}
			})
			r := frReconciler{
				client:  getFakeClient(t, feature.DeepCopy(), hr, sampleFeatureSet(feature.Spec.FeatureSet)),
				logger:  dummyLogger(),
				feature: feature.DeepCopy(),
			}
			err := r.reconcile(context.Background())
			assert.Nil(t, err)
			if assert.NotNil(t, r.feature.Status.Ready) {
				assert.Equal(t, tt.ready, *r.feature.Status.Ready)
			}
		})
	}
}

func TestUpdateFeatureSetEntry(t *testing.T) {
	testCases := map[string]struct {
		requireFeatures []string
		componentStatus []uiapi.ComponentStatus
		expectedErr     error
		expectedStatus  bool
	}{
		"Should return not found error when FeatureSet does not exist": {
			expectedErr: kerr.NewNotFound(
				schema.GroupResource{
					Group:    uiapi.SchemeGroupVersion.Group,
					Resource: uiapi.ResourceFeatureSets,
				},
				""),
		},
		"Should be disabled when all required features not enabled": {
			requireFeatures: []string{"foo", "bar"},
			componentStatus: []uiapi.ComponentStatus{
				{Name: "foo", Enabled: pointer.BoolP(true)},
			},
			expectedStatus: false,
		},
		"Should be enabled when all required features are enabled": {
			requireFeatures: []string{"foo", "bar"},
			componentStatus: []uiapi.ComponentStatus{
				{Name: "foo", Enabled: pointer.BoolP(true)},
				{Name: "bar", Enabled: pointer.BoolP(true)},
			},
			expectedStatus: true,
		},
		"Should be enabled when there are no require features and at least one feature enabled": {
			componentStatus: []uiapi.ComponentStatus{
				{Name: "foo", Enabled: pointer.BoolP(true)},
			},
			expectedStatus: true,
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			feature := sampleFeature()

			fs := sampleFeatureSet(feature.Spec.FeatureSet, func(in *uiapi.FeatureSet) {
				in.Spec.RequiredFeatures = tt.requireFeatures
				in.Status.Features = tt.componentStatus
			})
			initObjects := []client.Object{feature}
			if tt.expectedErr == nil {
				initObjects = append(initObjects, fs)
			}
			r := frReconciler{
				client:  getFakeClient(t, initObjects...),
				logger:  dummyLogger(),
				feature: feature,
			}
			err := r.reconcile(context.Background())
			assert.Nil(t, err)

			err = r.updateFeatureSetEntry(context.Background())
			if tt.expectedErr != nil {
				if assert.NotNil(t, err) {
					assert.True(t, kerr.IsNotFound(err))
				}
				return
			}
			assert.Nil(t, err)
			assert.Nil(t, r.client.Get(context.Background(), types.NamespacedName{Name: fs.Name}, fs))
			if assert.NotNil(t, fs.Status.Enabled) {
				assert.Equal(t, tt.expectedStatus, *fs.Status.Enabled)
			}
		})
	}
}

func TestFeatureSetGetDisabledWhenRequiredFeatureGetDeleted(t *testing.T) {
	curTime := metav1.Now()
	feature := sampleFeature(func(in *uiapi.Feature) {
		in.ObjectMeta.DeletionTimestamp = &curTime
	})

	fs := sampleFeatureSet(feature.Spec.FeatureSet, func(in *uiapi.FeatureSet) {
		in.Status.Features = []uiapi.ComponentStatus{
			{Name: feature.Name, Enabled: pointer.BoolP(true)},
		}
		in.Status.Enabled = pointer.BoolP(true)
	})
	r := frReconciler{
		client:  getFakeClient(t, feature, fs),
		logger:  dummyLogger(),
		feature: feature,
	}
	err := r.updateFeatureSetAndRemoveFinalizer(context.Background())
	assert.Nil(t, err)
	assert.Nil(t, r.client.Get(context.Background(), types.NamespacedName{Name: fs.Name}, fs))
	if assert.NotNil(t, fs.Status.Enabled) {
		assert.False(t, *fs.Status.Enabled)
	}
}

func getFakeClient(t *testing.T, initObjs ...client.Object) client.WithWatch {
	scheme := runtime.NewScheme()
	assert.Nil(t, uiapi.AddToScheme(scheme))
	assert.Nil(t, fluxcd.AddToScheme(scheme))

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjs...).Build()
}

func dummyLogger() logr.Logger {
	return logr.Discard()
}

func sampleFeature(transformFuncs ...func(in *uiapi.Feature)) *uiapi.Feature {
	fr := &uiapi.Feature{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-prometheus-stack",
		},
		Spec: uiapi.FeatureSpec{
			Title:       "Kube Prometheus Stack",
			Description: "lorem ipsum",
			FeatureSet:  "monitoring",
		},
	}
	for _, f := range transformFuncs {
		f(fr)
	}
	return fr
}

func sampleFeatureSet(name string, transformFuncs ...func(in *uiapi.FeatureSet)) *uiapi.FeatureSet {
	fs := &uiapi.FeatureSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: uiapi.FeatureSetSpec{
			Title:       name,
			Description: "lorem ipsum",
		},
	}
	for _, f := range transformFuncs {
		f(fs)
	}
	return fs
}

func sampleHelmRelease(transformFuncs ...func(in *fluxcd.HelmRelease)) *fluxcd.HelmRelease {
	hr := &fluxcd.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample-release",
			Namespace: "kubeops",
		},
		Spec: fluxcd.HelmReleaseSpec{},
	}

	for _, f := range transformFuncs {
		f(hr)
	}
	return hr
}
