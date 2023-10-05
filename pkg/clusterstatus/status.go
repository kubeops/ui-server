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
	"time"

	kmcluster "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type clusterStatusResponse struct {
	response *v1alpha1.ClusterStatusResponse
}

var clusterStatus *clusterStatusResponse

const pullingPeriod = time.Minute * 2

func init() {
	clusterStatus = &clusterStatusResponse{
		response: &v1alpha1.ClusterStatusResponse{},
	}
}

func GetClusterStatus() *v1alpha1.ClusterStatusResponse {
	return clusterStatus.response
}

func StartClusterStatusPuller(mgr manager.Manager) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		go func() {
			ticker := time.NewTicker(pullingPeriod)
			defer ticker.Stop()

			for {
				cs := generateClusterStatus(mgr)
				clusterStatus = cs
				<-ticker.C
			}
		}()

		return nil
	}
}

func generateClusterStatus(mgr manager.Manager) *clusterStatusResponse {
	csr := &clusterStatusResponse{
		response: &v1alpha1.ClusterStatusResponse{},
	}
	client := mgr.GetClient()
	var err error

	clusterManager := kmcluster.DetectClusterManager(client)
	csr.response.ClusterManagers = clusterManager.Strings()
	if clusterManager.ManagedByOCMMulticlusterControlplane() {
		csr.response.Phase = v1alpha1.ClusterPhaseActive
		return csr
	}

	csr.response.ClusterAPI, err = kmcluster.DetectCAPICluster(client)
	if err != nil {
		return csr
	}

	ready, msg, err := checkClusterReadiness(mgr.GetConfig())
	if err != nil {
		csr.assignStatusError(v1alpha1.ClusterPhaseInactive, v1alpha1.ClusterNotFound, err.Error())
		return csr
	} else if !ready {
		csr.assignStatusError(v1alpha1.ClusterPhaseNotReady, v1alpha1.MissingComponent, msg)
		return csr
	}
	csr.response.Phase = v1alpha1.ClusterPhaseActive

	return csr
}

func (cs *clusterStatusResponse) assignStatusError(phase v1alpha1.ClusterPhase, Reason v1alpha1.ClusterPhaseReason, Message string) {
	cs.response.Phase = phase
	cs.response.Reason = Reason
	cs.response.Message = Message
}
