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

	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	KeyACEManaged = "byte.builders/managed"
)

type FluxCDStatus struct {
	Installed bool   `json:"installed"`
	Ready     bool   `json:"ready"`
	Managed   bool   `json:"managed"`
	Message   string `json:"message,omitempty"`
}

func getFluxCDStatus(config *rest.Config) (FluxCDStatus, error) {
	status := FluxCDStatus{}
	if err := checkFluxCRDRegistered(config); err != nil {
		if kerr.IsNotFound(err) || meta.IsNoMatchError(err) {
			status.Installed = false
			status.Message = "FluxCD CRDs HelmReleases and HelmRepositories are not registered"
			return status, nil
		}
		return status, err
	}

	kc, err := kubernetes.NewForConfig(config)
	if err != nil {
		return status, err
	}

	srcCtrl, err := kc.AppsV1().Deployments(core.NamespaceAll).List(goctx.Background(), metav1.ListOptions{
		FieldSelector: "metadata.name=source-controller",
	})
	if err != nil {
		return status, err
	}
	if len(srcCtrl.Items) == 0 {
		status.Installed = false
		status.Message = "Deployment 'source-controller' does not exist"
		return status, nil
	}
	status.Installed = true
	status.Managed = isFluxCDManaged(srcCtrl.Items[0].Spec.Template.Labels)

	if srcCtrl.Items[0].Status.ReadyReplicas == 0 {
		status.Ready = false
		status.Message = "No ready replica found for deployment 'source-controller'"
		return status, nil
	}

	helmCtrl, err := kc.AppsV1().Deployments(core.NamespaceAll).List(goctx.Background(), metav1.ListOptions{
		FieldSelector: "metadata.name=helm-controller",
	})
	if err != nil {
		return status, err
	}
	if len(helmCtrl.Items) == 0 {
		status.Ready = false
		status.Message = "Deployment 'helm-controller' does not exist"
		return status, nil
	}
	if helmCtrl.Items[0].Status.ReadyReplicas == 0 {
		status.Ready = false
		status.Message = "No ready replica found for deployment 'helm-controller'"
	}
	status.Ready = true

	return status, nil
}

func isFluxCDManaged(podLabels map[string]string) bool {
	if _, ok := podLabels[KeyACEManaged]; ok {
		return true
	}
	return false
}

func checkFluxCRDRegistered(config *rest.Config) error {
	_, err := getResourceList(config, schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2beta1",
		Resource: "helmreleases",
	})
	if err != nil {
		return err
	}

	_, err = getResourceList(config, schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1beta1",
		Resource: "helmrepositories",
	})
	if err != nil {
		return err
	}
	return nil
}
