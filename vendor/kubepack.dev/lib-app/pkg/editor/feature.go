/*
Copyright AppsCode Inc. and Contributors

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

package editor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/pkg/errors"
	kj "gomodules.xyz/encoding/json"
	"helm.sh/helm/v3/pkg/chart"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	"kmodules.xyz/resource-metadata/hub"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func UpdateFeatureValues(kc client.Client, chrt *chart.Chart, vals map[string]any) (map[string]any, error) {
	var gvr metav1.GroupVersionResource

	if data, ok := chrt.Metadata.Annotations["meta.x-helm.dev/editor"]; ok && data != "" {
		if err := json.Unmarshal([]byte(data), &gvr); err != nil {
			return nil, err
		}
	} else {
		return vals, nil
	}

	fsGVR := metav1.GroupVersionResource{
		Group:    "ui.k8s.appscode.com",
		Version:  "v1alpha1",
		Resource: "featuresets",
	}
	if gvr != fsGVR {
		return vals, nil
	}

	if resources, ok, err := unstructured.NestedMap(vals, "resources"); err == nil && ok {
		for k, o := range resources {
			// helmToolkitFluxcdIoHelmRelease_kubestash
			if !strings.HasPrefix(k, "helmToolkitFluxcdIoHelmRelease_") {
				continue
			}

			obj := o.(map[string]interface{})
			featureName, found, err := unstructured.NestedString(obj, "metadata", "name")
			if err != nil {
				return nil, errors.Wrap(err, "can't detect feature name")
			} else if !found {
				return nil, fmt.Errorf("feature name not found for key %s", k)
			}

			var feature uiapi.Feature
			err = kc.Get(context.TODO(), client.ObjectKey{Name: featureName}, &feature)
			if err == nil {
				err = SetChartInfo(kc, &feature, k, vals)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return vals, nil
}

func SetChartInfo(kc client.Client, feature *uiapi.Feature, featureKey string, values map[string]interface{}) error {
	err := unstructured.SetNestedField(values, feature.Spec.Chart.Name, "resources", featureKey, "spec", "chart", "spec", "chart")
	if err != nil {
		return err
	}
	if feature.Spec.Chart.Version != "" {
		err = unstructured.SetNestedField(values, feature.Spec.Chart.Version, "resources", featureKey, "spec", "chart", "spec", "version")
		if err != nil {
			return err
		}
	} else {
		unstructured.RemoveNestedField(values, "resources", featureKey, "spec", "chart", "spec", "version")
	}
	err = unstructured.SetNestedField(values, feature.Spec.Chart.SourceRef.Kind, "resources", featureKey, "spec", "chart", "spec", "sourceRef", "kind")
	if err != nil {
		return err
	}
	err = unstructured.SetNestedField(values, feature.Spec.Chart.SourceRef.Name, "resources", featureKey, "spec", "chart", "spec", "sourceRef", "name")
	if err != nil {
		return err
	}
	err = unstructured.SetNestedField(values, feature.Spec.Chart.SourceRef.Namespace, "resources", featureKey, "spec", "chart", "spec", "sourceRef", "namespace")
	if err != nil {
		return err
	}

	if len(feature.Spec.Chart.ValuesFiles) > 0 {
		valuesFiles, err := kj.ToJsonArray(feature.Spec.Chart.ValuesFiles)
		if err != nil {
			return err
		}
		if err := unstructured.SetNestedField(values, valuesFiles, "resources", featureKey, "spec", "chart", "spec", "valuesFiles"); err != nil {
			return err
		}
	}

	err = unstructured.SetNestedField(values, feature.Spec.Chart.Namespace, "resources", featureKey, "spec", "targetNamespace")
	if err != nil {
		return err
	}
	err = unstructured.SetNestedField(values, feature.Spec.Chart.Namespace, "resources", featureKey, "spec", "storageNamespace")
	if err != nil {
		return err
	}

	err = unstructured.SetNestedField(values, feature.Spec.Chart.CreateNamespace, "resources", featureKey, "spec", "install", "createNamespace")
	if err != nil {
		return err
	}

	var hr v2.HelmRelease
	err = kc.Get(context.Background(), types.NamespacedName{Name: feature.Name, Namespace: hub.BootstrapHelmRepositoryNamespace()}, &hr)
	if err == nil {
		if hr.Spec.Values != nil {
			if err = setFeatureValues(values, hr.Spec.Values.Raw, featureKey); err != nil {
				return err
			}
		}
	} else if apierrors.IsNotFound(err) {
		if feature.Spec.Values != nil {
			if err = setFeatureValues(values, feature.Spec.Values.Raw, featureKey); err != nil {
				return err
			}
		}
	} else {
		return err
	}

	if len(feature.Spec.ValuesFrom) > 0 {
		valuesFrom, err := kj.ToJsonArray(feature.Spec.ValuesFrom)
		if err != nil {
			return err
		}
		if err := unstructured.SetNestedField(values, valuesFrom, "resources", featureKey, "spec", "valuesFrom"); err != nil {
			return err
		}
	}

	return nil
}

func setFeatureValues(values map[string]interface{}, specValues []byte, featureKey string) error {
	featureValues := map[string]interface{}{}
	if err := json.Unmarshal(specValues, &featureValues); err != nil {
		return err
	}
	return unstructured.SetNestedField(values, featureValues, "resources", featureKey, "spec", "values")
}
