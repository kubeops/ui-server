/*
Copyright AppsCode Inc. and Contributors

Licensed under the AppsCode Community License 1.0.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://github.com/appscode/licenses/raw/1.0.0/AppsCode-Community-1.0.0.md

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gateway

import (
	"context"

	catgwapi "go.bytebuilders.dev/catalog/api/gateway/v1alpha1"

	flux "github.com/fluxcd/helm-controller/api/v2"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kmapi "kmodules.xyz/client-go/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func WaitIfNeeded(ctx context.Context, kc client.Client, bindingNamespace string) (bool, error) {
	var ns core.Namespace
	err := kc.Get(ctx, client.ObjectKey{Name: bindingNamespace}, &ns)
	if err != nil {
		return false, err
	}

	// if client-org, check if there should be a helmRelease on the <>-gw ns. And then check that hr's existence
	if ns.Labels[kmapi.ClientOrgKey] == "true" {
		willBe := willThereBeAHelmRelease(ctx, kc, bindingNamespace)
		if willBe {
			var hr flux.HelmRelease
			err = kc.Get(ctx, types.NamespacedName{
				Namespace: bindingNamespace + "-gw",
				Name:      bindingNamespace,
			}, &hr)
			// if not found, wait.
			if err != nil {
				return errors.IsNotFound(err), err
			}
			// if found, wait for that to be ready
			return !isHelmReleaseReady(hr), nil
		}
	}
	return false, nil
}

func willThereBeAHelmRelease(ctx context.Context, kc client.Client, ns string) bool {
	var gwps catgwapi.GatewayPreset
	if err := kc.Get(ctx, types.NamespacedName{
		Namespace: ns + "-gw",
		Name:      ns,
	}, &gwps); err != nil {
		return false
	}

	var gwcfg catgwapi.GatewayConfig
	err := kc.Get(ctx, types.NamespacedName{
		Namespace: string(*gwps.Spec.ParametersRef.Namespace),
		Name:      gwps.Spec.ParametersRef.Name,
	}, &gwcfg)
	return err == nil
}

func isHelmReleaseReady(hr flux.HelmRelease) bool {
	conditions := hr.Status.Conditions
	for i := range conditions {
		if conditions[i].Type == "Ready" && conditions[i].Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}
