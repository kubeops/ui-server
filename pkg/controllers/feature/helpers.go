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
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"go.openviz.dev/apimachinery/apis/openviz/v1alpha1"
	"gomodules.xyz/pointer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"kmodules.xyz/client-go/meta"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
)

type Status struct {
	Replicas          int32              `json:"replicas,omitempty"`
	ReadyReplicas     int32              `json:"readyReplicas,omitempty"`
	CurrentReplicas   int32              `json:"currentReplicas,omitempty"`
	UpdatedReplicas   int32              `json:"updatedReplicas,omitempty"`
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	AvailableReplicas int32              `json:"availableReplicas,omitempty"`
}

func isRequiredResourcesExist(status featureStatus) bool {
	if status.resources != nil && !status.resources.found {
		return false
	}
	return true
}

func isWorkloadOrReleaseExist(status featureStatus) bool {
	if status.workload != nil && status.workload.found {
		return true
	}
	if status.release != nil && status.release.found {
		return true
	}
	return false
}

func isWorkLoadsReady(objList unstructured.UnstructuredList) bool {
	for idx := range objList.Items {
		obj := objList.Items[idx]
		status := Status{}
		statusData, ok := obj.UnstructuredContent()["status"]
		if !ok {
			return false
		}
		if err := ParseInto(statusData, &status); err != nil {
			return false
		}

		switch obj.GroupVersionKind().Kind {
		case v1alpha1.ResourceKindGrafanaDashboard, "Pod":
			if !meta.IsConditionTrue(status.Conditions, "Ready") {
				return false
			}
		case "Deployment", "StatefulSet":
			if status.ReadyReplicas != status.Replicas {
				return false
			}
		}
	}
	return true
}

func getAllEnabledFeatures(fs *uiapi.FeatureSet) []string {
	var features []string
	for _, f := range fs.Status.Features {
		if pointer.Bool(f.Enabled) {
			features = append(features, f.Name)
		}
	}
	return features
}

func allEnabledFeaturesReady(fs *uiapi.FeatureSet) (enabled bool, reason string) {
	features := getAllEnabledFeatures(fs)
	for _, f := range features {
		if !isFeatureReady(f, fs.Status.Features) {
			return false, fmt.Sprintf("Feature '%s' is enabled but not ready.", f)
		}
	}
	return true, ""
}

func isFeatureReady(featureName string, status []uiapi.ComponentStatus) bool {
	for i := range status {
		if status[i].Name == featureName && (status[i].Ready != nil && *status[i].Ready) {
			return true
		}
	}
	return false
}

func atLeastOneFeatureEnabled(status []uiapi.ComponentStatus) bool {
	for i := range status {
		if pointer.Bool(status[i].Enabled) {
			return true
		}
	}
	return false
}

func getRandomInterval() time.Duration {
	minSecond := 30
	maxSecond := 120
	offset := rand.Int() % maxSecond
	return time.Second * time.Duration(minSecond+offset)
}

func ParseInto(src any, dst any) error {
	jsonByte, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonByte, dst)
}
