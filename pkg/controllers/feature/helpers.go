package feature

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"go.openviz.dev/apimachinery/apis/openviz/v1alpha1"
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

func findReason(status featureStatus) string {
	if status.resources != nil && !status.resources.found {
		return status.resources.reason
	}
	if status.workload != nil && !status.workload.found {
		return status.workload.reason
	}
	return "No relevant resources found for the Feature"
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

func isReleaseReady(conditions []metav1.Condition) bool {
	for i := range conditions {
		if conditions[i].Type == "Ready" && conditions[i].Status == "True" {
			return true
		}
	}
	return false
}

func allRequireFeaturesReady(fs *uiapi.FeatureSet) (enabled bool, reason string) {
	for _, f := range fs.Spec.RequiredFeatures {
		if !isFeatureReady(f, fs.Status.Features) {
			return false, fmt.Sprintf("Required feature '%s' is not ready.", f)
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

func atLeastOneFeatureManaged(status []uiapi.ComponentStatus) bool {
	for i := range status {
		if status[i].Enabled != nil && *status[i].Enabled &&
			status[i].Managed != nil && *status[i].Managed {
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
