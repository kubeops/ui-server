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

	"gomodules.xyz/pointer"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	clustermeta "kmodules.xyz/client-go/cluster"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MessageFluxCDMissing             = "FluxCD is not installed or not ready. Please, reconnect to install/update the component."
	MessageRequiredComponentsMissing = "Kube-ui-server is not ready. Please, reconnect to update the components."
)

func generateClusterStatusResponse(kc client.Client, mapper meta.RESTMapper) *rsapi.ClusterStatusResponse {
	var csr rsapi.ClusterStatusResponse
	var err error

	clusterManager := clustermeta.DetectClusterManager(kc, mapper)
	csr.ClusterManagers = clusterManager.Strings()
	if clusterManager.ManagedByOCMMulticlusterControlplane() {
		csr.Phase = rsapi.ClusterPhaseActive
		return &csr
	}

	csr.ClusterAPI, err = clustermeta.DetectCAPICluster(kc)
	if err != nil {
		return &csr
	}

	ready, msg, err := checkClusterReadiness(kc)
	if err != nil {
		csr.Phase = rsapi.ClusterPhaseInactive
		csr.Reason = rsapi.ClusterPhaseReasonClusterNotFound
		csr.Message = err.Error()
		return &csr
	} else if !ready {
		csr.Phase = rsapi.ClusterPhaseNotReady
		csr.Reason = rsapi.ClusterPhaseReasonMissingComponent
		csr.Message = msg
		return &csr
	}
	csr.Phase = rsapi.ClusterPhaseActive

	return &csr
}

func checkClusterReadiness(kc client.Client) (bool, string, error) {
	ready, err := isFluxCDReady(kc)
	if err != nil {
		return false, "", err
	}

	if !ready {
		return false, MessageFluxCDMissing, nil
	}

	ready, err = isKubeUiServerReady(kc)
	if err != nil {
		return false, "", err
	}
	if !ready {
		return false, MessageRequiredComponentsMissing, nil
	}
	return true, "", nil
}

func isKubeUiServerReady(kc client.Client) (bool, error) {
	var feature uiapi.Feature
	err := kc.Get(context.TODO(), types.NamespacedName{Name: "kube-ui-server"}, &feature)
	if err != nil {
		return false, err
	}

	if !pointer.Bool(feature.Status.Ready) {
		return false, nil
	}
	return true, nil
}

func isFluxCDReady(kc client.Client) (bool, error) {
	status, err := getFluxCDStatus(kc)
	if err != nil {
		return false, err
	}
	return status.Ready, nil
}
