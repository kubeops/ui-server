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
	"context"

	apps "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FluxCDStatus struct {
	Installed bool   `json:"installed"`
	Ready     bool   `json:"ready"`
	Managed   bool   `json:"managed"`
	Message   string `json:"message,omitempty"`
}

func getFluxCDStatus(kc client.Client) (FluxCDStatus, error) {
	status := FluxCDStatus{}
	if err := checkFluxCRDRegistered(kc.RESTMapper()); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			status.Installed = false
			status.Message = "FluxCD CRDs HelmReleases and HelmRepositories are not registered"
			return status, nil
		}
		return status, err
	}

	var deployments apps.DeploymentList
	err := kc.List(context.Background(), &deployments, client.MatchingLabels{
		"app.kubernetes.io/instance": "flux-system",
		"app.kubernetes.io/part-of":  "flux",
		"control-plane":              "controller",
	})
	if err != nil {
		return status, err
	}
	if len(deployments.Items) == 0 {
		status.Installed = false
		status.Message = "FluxCD deployments do not exist"
		return status, nil
	}

	for _, deploy := range deployments.Items {
		switch deploy.Name {
		case "source-controller":
			status.Installed = true
			status.Managed = isFluxCDManaged(deploy.Spec.Template.Labels)

			if deploy.Status.ReadyReplicas == 0 {
				status.Ready = false
				status.Message = "No ready replica found for deployment 'source-controller'"
				return status, nil
			}
		case "helm-controller":
			if deploy.Status.ReadyReplicas == 0 {
				status.Ready = false
				status.Message = "No ready replica found for deployment 'helm-controller'"
			}
		}
	}
	status.Ready = true

	return status, nil
}

func isFluxCDManaged(podLabels map[string]string) bool {
	_, exists := podLabels[rsapi.KeyACEManaged]
	return exists
}

func checkFluxCRDRegistered(mapper meta.RESTMapper) error {
	if _, err := mapper.RESTMappings(schema.GroupKind{
		Group: "helm.toolkit.fluxcd.io",
		Kind:  "HelmRelease",
	}); err != nil {
		return err
	}
	if _, err := mapper.RESTMappings(schema.GroupKind{
		Group: "source.toolkit.fluxcd.io",
		Kind:  "HelmRepository",
	}); err != nil {
		return err
	}
	return nil
}
