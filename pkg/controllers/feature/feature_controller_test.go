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
	"testing"

	fluxhelm "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"gomodules.xyz/pointer"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	meta_util "kmodules.xyz/client-go/meta"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testFeatureSetName = "opscenter-monitoring"
	testFeatureName    = "kube-prometheus-stack"
)

var testWorkloadLabels = map[string]string{
	"app.kubernetes.io/managed-by": "Helm",
	"app.kubernetes.io/name":       "sample-workload",
}

func TestFeatureEnableStatus(t *testing.T) {
	testCases := map[string]struct {
		feature               *uiapi.Feature
		workload              *apps.Deployment
		helmRelease           *fluxhelm.HelmRelease
		expectedEnabledStatus bool
		errorExpected         bool
	}{
		"Should be false when required resources does not exist": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.ReadinessChecks.Resources = []metav1.GroupVersionKind{
					{
						Group:   "foo.io",
						Version: "v1",
						Kind:    "Foo",
					},
				}
			}),
			expectedEnabledStatus: false,
		},
		"Should be true when required workload exist": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.ReadinessChecks.Workloads = []uiapi.WorkloadInfo{
					{
						GroupVersionKind: metav1.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Selector: testWorkloadLabels,
					},
				}
			}),
			workload:              sampleDeployment(),
			expectedEnabledStatus: true,
		},
		"Should be true when required workload does not exist but HelmRelease exist": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.ReadinessChecks.Workloads = []uiapi.WorkloadInfo{
					{
						GroupVersionKind: metav1.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Selector: testWorkloadLabels,
					},
				}
			}),
			helmRelease:           sampleHelmRelease(),
			expectedEnabledStatus: true,
		},
		"Should be false when neither workload nor HelmRelease exist": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.ReadinessChecks.Workloads = []uiapi.WorkloadInfo{
					{
						GroupVersionKind: metav1.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Selector: testWorkloadLabels,
					},
				}
			}),
			expectedEnabledStatus: false,
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			initObjects := []client.Object{tt.feature}
			if tt.workload != nil {
				initObjects = append(initObjects, tt.workload)
			}
			if tt.helmRelease != nil {
				initObjects = append(initObjects, tt.helmRelease)
			}
			r := frReconciler{
				client:  getFakeClient(t, initObjects...),
				logger:  dummyLogger(),
				feature: tt.feature,
			}
			r.apiReader = r.client

			err := r.reconcile(context.Background())
			if tt.errorExpected {
				assert.NotNil(t, err)
				assert.NotEmpty(t, r.feature.Status.Note)
				return
			}

			if !assert.Nil(t, err) {
				return
			}
			if !assert.NotNil(t, r.feature.Status.Enabled) {
				return
			}
			assert.Equal(t, tt.expectedEnabledStatus, *r.feature.Status.Enabled)
		})
	}
}

func TestFeatureReadyStatus(t *testing.T) {
	testCases := map[string]struct {
		feature             *uiapi.Feature
		workload            *apps.Deployment
		helmRelease         *fluxhelm.HelmRelease
		expectedReadyStatus bool
		errorExpected       bool
	}{
		"Should not be ready when dependency is not satisfied": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.Requirements.Features = []string{"foo", "bar"}
				in.Spec.ReadinessChecks.Workloads = []uiapi.WorkloadInfo{
					{
						GroupVersionKind: metav1.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Selector: testWorkloadLabels,
					},
				}
			}),
			workload:            sampleDeployment(),
			helmRelease:         sampleHelmRelease(),
			expectedReadyStatus: false,
		},
		"Should not be ready when the workload does not exit": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.ReadinessChecks.Workloads = []uiapi.WorkloadInfo{
					{
						GroupVersionKind: metav1.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Selector: testWorkloadLabels,
					},
				}
			}),
			helmRelease:         sampleHelmRelease(),
			expectedReadyStatus: false,
		},
		"Should not be ready when workload exist but HelmRelease does not exist": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.ReadinessChecks.Workloads = []uiapi.WorkloadInfo{
					{
						GroupVersionKind: metav1.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Selector: testWorkloadLabels,
					},
				}
			}),
			workload:            sampleDeployment(),
			expectedReadyStatus: false,
		},
		"Should not be ready when workload exist but HelmRelease is not ready ": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.ReadinessChecks.Workloads = []uiapi.WorkloadInfo{
					{
						GroupVersionKind: metav1.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Selector: testWorkloadLabels,
					},
				}
			}),
			workload: sampleDeployment(),
			helmRelease: sampleHelmRelease(func(in *fluxhelm.HelmRelease) {
				in.Status.Conditions = []metav1.Condition{
					{
						Type:   "Ready",
						Status: "False",
					},
				}
			}),
			expectedReadyStatus: false,
		},
		"Should be ready when workload exist and HelmRelease is ready": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.ReadinessChecks.Workloads = []uiapi.WorkloadInfo{
					{
						GroupVersionKind: metav1.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Selector: testWorkloadLabels,
					},
				}
			}),
			workload: sampleDeployment(),
			helmRelease: sampleHelmRelease(func(in *fluxhelm.HelmRelease) {
				in.Status.Conditions = []metav1.Condition{
					{
						Type:   "Ready",
						Status: "True",
					},
				}
			}),
			expectedReadyStatus: true,
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			initObjects := []client.Object{tt.feature}
			if tt.workload != nil {
				initObjects = append(initObjects, tt.workload)
			}
			if tt.helmRelease != nil {
				initObjects = append(initObjects, tt.helmRelease)
			}
			r := frReconciler{
				client:  getFakeClient(t, initObjects...),
				logger:  dummyLogger(),
				feature: tt.feature,
			}
			r.apiReader = r.client

			err := r.reconcile(context.Background())
			if tt.errorExpected {
				assert.NotNil(t, err)
				assert.NotEmpty(t, r.feature.Status.Note)
				return
			}

			if !assert.Nil(t, err) {
				return
			}
			if !assert.NotNil(t, r.feature.Status.Enabled) {
				return
			}
			assert.Equal(t, true, *r.feature.Status.Enabled)

			if !tt.expectedReadyStatus {
				if r.feature.Status.Ready != nil {
					assert.Equal(t, tt.expectedReadyStatus, *r.feature.Status.Ready)
				}
				return
			}
			if !assert.NotNil(t, r.feature.Status.Ready) {
				return
			}
			assert.Equal(t, tt.expectedReadyStatus, *r.feature.Status.Ready)
		})
	}
}

func TestFeatureManagedStatus(t *testing.T) {
	testCases := map[string]struct {
		feature               *uiapi.Feature
		workload              *apps.Deployment
		helmRelease           *fluxhelm.HelmRelease
		expectedManagedStatus bool
		errorExpected         bool
	}{
		"Managed should be true when HelmRelease exist": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.ReadinessChecks.Workloads = []uiapi.WorkloadInfo{
					{
						GroupVersionKind: metav1.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Selector: testWorkloadLabels,
					},
				}
			}),
			workload:              sampleDeployment(),
			helmRelease:           sampleHelmRelease(),
			expectedManagedStatus: true,
		},
		"Managed should be false when HelmRelease does not exist": {
			feature: sampleFeature(func(in *uiapi.Feature) {
				in.Spec.ReadinessChecks.Workloads = []uiapi.WorkloadInfo{
					{
						GroupVersionKind: metav1.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Selector: testWorkloadLabels,
					},
				}
			}),
			workload:              sampleDeployment(),
			expectedManagedStatus: false,
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			initObjects := []client.Object{tt.feature}
			if tt.workload != nil {
				initObjects = append(initObjects, tt.workload)
			}
			if tt.helmRelease != nil {
				initObjects = append(initObjects, tt.helmRelease)
			}
			r := frReconciler{
				client:  getFakeClient(t, initObjects...),
				logger:  dummyLogger(),
				feature: tt.feature,
			}
			r.apiReader = r.client

			err := r.reconcile(context.Background())
			if tt.errorExpected {
				assert.NotNil(t, err)
				assert.NotEmpty(t, r.feature.Status.Note)
				return
			}

			if !assert.Nil(t, err) {
				return
			}
			if !assert.NotNil(t, r.feature.Status.Enabled) {
				return
			}
			assert.Equal(t, true, *r.feature.Status.Enabled)

			if !tt.expectedManagedStatus {
				if r.feature.Status.Managed != nil {
					assert.Equal(t, tt.expectedManagedStatus, *r.feature.Status.Managed)
				}
				return
			}
			if !assert.NotNil(t, r.feature.Status.Managed) {
				return
			}
			assert.Equal(t, tt.expectedManagedStatus, *r.feature.Status.Managed)
		})
	}
}

func TestFeatureSetStatus(t *testing.T) {
	testCases := map[string]struct {
		requireFeatures      []string
		componentStatus      []uiapi.ComponentStatus
		expectedErr          error
		expectedEnableStatus *bool
		expectedReadyStatus  *bool
	}{
		"Should return not found error when FeatureSet does not exist": {
			expectedErr: kerr.NewNotFound(
				schema.GroupResource{
					Group:    uiapi.SchemeGroupVersion.Group,
					Resource: uiapi.ResourceFeatureSets,
				},
				""),
		},
		"Should not be enabled when no features are enabled": {
			requireFeatures: []string{"foo", "bar"},
			componentStatus: []uiapi.ComponentStatus{
				{Name: "foo", Enabled: pointer.BoolP(false), Managed: nil, Ready: nil},
				{Name: "bar", Enabled: pointer.BoolP(false), Managed: nil, Ready: nil},
				{Name: "baz", Enabled: pointer.BoolP(false), Managed: nil, Ready: nil},
			},
			expectedEnableStatus: pointer.BoolP(false),
			expectedReadyStatus:  pointer.BoolP(false),
		},
		"Should not be enabled when there is no managed features": {
			requireFeatures: []string{"foo", "bar"},
			componentStatus: []uiapi.ComponentStatus{
				{Name: "foo", Enabled: pointer.BoolP(true), Managed: pointer.BoolP(false), Ready: nil},
				{Name: "bar", Enabled: pointer.BoolP(true), Managed: pointer.BoolP(false), Ready: nil},
				{Name: "baz", Enabled: pointer.BoolP(true), Managed: pointer.BoolP(false), Ready: nil},
			},
			expectedEnableStatus: pointer.BoolP(false),
			expectedReadyStatus:  pointer.BoolP(false),
		},
		"Should be enabled when at least one managed feature is enabled": {
			requireFeatures: []string{"foo", "bar"},
			componentStatus: []uiapi.ComponentStatus{
				{Name: "foo", Enabled: pointer.BoolP(true), Managed: pointer.BoolP(true), Ready: pointer.BoolP(false)},
				{Name: "bar", Enabled: pointer.BoolP(false), Managed: pointer.BoolP(true), Ready: nil},
				{Name: "baz", Enabled: pointer.BoolP(false), Managed: pointer.BoolP(false), Ready: nil},
			},
			expectedEnableStatus: pointer.BoolP(true),
			expectedReadyStatus:  pointer.BoolP(false),
		},
		"Should not be ready when all required features are not ready": {
			requireFeatures: []string{"foo", "bar"},
			componentStatus: []uiapi.ComponentStatus{
				{Name: "foo", Enabled: pointer.BoolP(true), Managed: pointer.BoolP(true), Ready: pointer.BoolP(true)},
				{Name: "bar", Enabled: pointer.BoolP(true), Managed: pointer.BoolP(true), Ready: pointer.BoolP(false)},
				{Name: "baz", Enabled: pointer.BoolP(false), Managed: pointer.BoolP(false), Ready: nil},
			},
			expectedEnableStatus: pointer.BoolP(true),
			expectedReadyStatus:  pointer.BoolP(false),
		},
		"Should be ready when all the required features are ready": {
			requireFeatures: []string{"foo", "bar"},
			componentStatus: []uiapi.ComponentStatus{
				{Name: "foo", Enabled: pointer.BoolP(true), Managed: pointer.BoolP(true), Ready: pointer.BoolP(true)},
				{Name: "bar", Enabled: pointer.BoolP(true), Managed: pointer.BoolP(true), Ready: pointer.BoolP(true)},
				{Name: "baz", Enabled: pointer.BoolP(false), Managed: pointer.BoolP(false), Ready: nil},
			},
			expectedEnableStatus: pointer.BoolP(true),
			expectedReadyStatus:  pointer.BoolP(true),
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
			if !assert.Nil(t, err) {
				return
			}
			assert.Nil(t, r.client.Get(context.Background(), types.NamespacedName{Name: fs.Name}, fs))

			if tt.expectedEnableStatus == nil {
				assert.Nil(t, fs.Status.Enabled)
				return
			}
			if !assert.NotNil(t, fs.Status.Enabled) {
				return
			}
			assert.Equal(t, *tt.expectedEnableStatus, *fs.Status.Enabled)

			if tt.expectedReadyStatus == nil {
				assert.Nil(t, fs.Status.Ready)
				return
			}
			if !assert.NotNil(t, fs.Status.Ready) {
				return
			}
			assert.Equal(t, *tt.expectedReadyStatus, *fs.Status.Ready)
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
	assert.Nil(t, fluxhelm.AddToScheme(scheme))
	assert.Nil(t, apps.AddToScheme(scheme))

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjs...).Build()
}

func dummyLogger() logr.Logger {
	return logr.Discard()
}

func sampleFeature(transformFuncs ...func(in *uiapi.Feature)) *uiapi.Feature {
	fr := &uiapi.Feature{
		ObjectMeta: metav1.ObjectMeta{
			Name: testFeatureName,
		},
		Spec: uiapi.FeatureSpec{
			Title:       "Kube Prometheus Stack",
			Description: "lorem ipsum",
			FeatureSet:  testFeatureSetName,
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

func sampleHelmRelease(transformFuncs ...func(in *fluxhelm.HelmRelease)) *fluxhelm.HelmRelease {
	hr := &fluxhelm.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample-helm-release",
			Namespace: "kubeops",
			Labels: map[string]string{
				meta_util.ComponentLabelKey: testFeatureName,
				meta_util.PartOfLabelKey:    testFeatureSetName,
			},
		},
		Spec: fluxhelm.HelmReleaseSpec{},
	}

	for _, f := range transformFuncs {
		f(hr)
	}
	return hr
}

func sampleDeployment() *apps.Deployment {
	return &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample-deployment",
			Namespace: "default",
			Labels:    testWorkloadLabels,
		},
		Spec: apps.DeploymentSpec{
			Replicas: pointer.Int32P(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: testWorkloadLabels,
			},
			Template: core.PodTemplateSpec{},
		},
	}
}
