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

package clusterstatus

import (
	goctx "context"

	"gomodules.xyz/pointer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
)

const (
	MessageFluxCDMissing             = "FluxCD is not installed or not ready. Please, reconnect to install/update the component."
	MessageRequiredComponentsMissing = "One or more core components are not ready. Please, reconnect to update the components."
)

func checkClusterReadiness(config *rest.Config) (bool, string, error) {
	ready, err := isFluxCDReady(config)
	if err != nil {
		return false, "", err
	}

	if !ready {
		return false, MessageFluxCDMissing, nil
	}

	ready, err = areRequiredFeatureSetsReady(config)
	if err != nil {
		return false, "", err
	}
	if !ready {
		return false, MessageRequiredComponentsMissing, nil
	}
	return true, "", nil
}

func areRequiredFeatureSetsReady(config *rest.Config) (bool, error) {
	featureSets, err := getResourceList(config, uiapi.SchemeGroupVersion.WithResource(uiapi.ResourceFeatureSets))
	if err != nil {
		return false, err
	}
	if len(featureSets.Items) == 0 {
		return false, nil
	}

	for _, fs := range featureSets.Items {
		featureSet := uiapi.FeatureSet{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(fs.UnstructuredContent(), &featureSet)
		if err != nil {
			return false, err
		}
		if len(featureSet.Spec.RequiredFeatures) > 0 && !pointer.Bool(featureSet.Status.Ready) {
			return false, nil
		}
	}
	return true, nil
}

func isFluxCDReady(config *rest.Config) (bool, error) {
	status, err := getFluxCDStatus(config)
	if err != nil {
		return false, err
	}
	return status.Ready, nil
}

func getResourceList(config *rest.Config, gvr schema.GroupVersionResource) (*unstructured.UnstructuredList, error) {
	dc, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return dc.Resource(gvr).List(goctx.Background(), metav1.ListOptions{})
}
