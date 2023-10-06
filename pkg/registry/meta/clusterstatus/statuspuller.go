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

	"gomodules.xyz/pointer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	kmcluster "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	uiapi "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type clusterStatusResponse struct {
	response *v1alpha1.ClusterStatusResponse
}

var clusterStatus *clusterStatusResponse

const (
	pullingPeriod = time.Minute * 2

	MessageFluxCDMissing             = "FluxCD is not installed or not ready. Please, reconnect to install/update the component."
	MessageRequiredComponentsMissing = "One or more core components are not ready. Please, reconnect to update the components."
)

func init() {
	clusterStatus = &clusterStatusResponse{
		response: &v1alpha1.ClusterStatusResponse{},
	}
}

func GetClusterStatus() *v1alpha1.ClusterStatusResponse {
	return clusterStatus.response
}

func (cs *clusterStatusResponse) assignStatus(phase v1alpha1.ClusterPhase, Reason v1alpha1.ClusterPhaseReason, Message string) {
	cs.response.Phase = phase
	cs.response.Reason = Reason
	cs.response.Message = Message
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

	ready, msg, err := checkClusterReadiness(mgr)
	if err != nil {
		csr.assignStatus(v1alpha1.ClusterPhaseInactive, v1alpha1.ClusterNotFound, err.Error())
		return csr
	} else if !ready {
		csr.assignStatus(v1alpha1.ClusterPhaseNotReady, v1alpha1.MissingComponent, msg)
		return csr
	}
	csr.response.Phase = v1alpha1.ClusterPhaseActive

	return csr
}

func checkClusterReadiness(mgr manager.Manager) (bool, string, error) {
	ready, err := isFluxCDReady(mgr)
	if err != nil {
		return false, "", err
	}

	if !ready {
		return false, MessageFluxCDMissing, nil
	}

	ready, err = areRequiredFeatureSetsReady(mgr)
	if err != nil {
		return false, "", err
	}
	if !ready {
		return false, MessageRequiredComponentsMissing, nil
	}
	return true, "", nil
}

func areRequiredFeatureSetsReady(mgr manager.Manager) (bool, error) {
	featureSets, err := getResourceList(mgr.GetConfig(), uiapi.SchemeGroupVersion.WithResource(uiapi.ResourceFeatureSets))
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

func isFluxCDReady(mgr manager.Manager) (bool, error) {
	status, err := getFluxCDStatus(mgr)
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
	return dc.Resource(gvr).List(context.Background(), metav1.ListOptions{})
}
