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

package v1alpha1

import (
	core "k8s.io/api/core/v1"
	resourcemetrics "kmodules.xyz/resource-metrics"
	"kmodules.xyz/resource-metrics/api"
	"reflect"
)

func init() {
	RegisterPathMapperPluginMembersWithApiPlugin(OpsResourceCalculator{}.ResourceCalculator())
}

type OpsResourceCalculator struct{}

func (r OpsResourceCalculator) ResourceCalculator() api.ResourceCalculator {
	return &api.ResourceCalculatorFuncs{
		AppRoles:               []api.PodRole{api.PodRoleDefault},
		RuntimeRoles:           []api.PodRole{api.PodRoleDefault, api.PodRoleExporter},
		RoleReplicasFn:         r.roleReplicasFn,
		ModeFn:                 r.modeFn,
		UsesTLSFn:              r.usesTLSFn,
		RoleResourceLimitsFn:   r.roleResourceFn(api.ResourceLimits),
		RoleResourceRequestsFn: r.roleResourceFn(api.ResourceRequests),
	}
}

func (r OpsResourceCalculator) roleReplicasFn(opsObj map[string]interface{}) (api.ReplicaList, error) {
	scaledObject, err := GetScaledObject(opsObj)
	if err != nil {
		return nil, err
	}
	result, err := resourcemetrics.RoleReplicas(scaledObject)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (r OpsResourceCalculator) modeFn(opsObj map[string]interface{}) (string, error) {
	scaledObject, err := GetScaledObject(opsObj)
	if err != nil {
		return "", err
	}
	mode, err := resourcemetrics.Mode(scaledObject)
	if err != nil {
		return "", err
	}

	return mode, nil
}

func (r OpsResourceCalculator) usesTLSFn(opsObj map[string]interface{}) (bool, error) {
	scaledObject, err := GetScaledObject(opsObj)
	if err != nil {
		return false, err
	}
	isUsedTLS, err := resourcemetrics.UsesTLS(scaledObject)
	if err != nil {
		return false, err
	}

	return isUsedTLS, nil
}

func (r OpsResourceCalculator) roleResourceFn(fn func(rr core.ResourceRequirements) core.ResourceList) func(opsObj map[string]interface{}) (map[api.PodRole]core.ResourceList, error) {
	return func(opsObj map[string]interface{}) (map[api.PodRole]core.ResourceList, error) {
		var rs map[api.PodRole]core.ResourceList
		var err error

		scaledObject, err := GetScaledObject(opsObj)
		if err != nil {
			return nil, err
		}
		// TODO: check if its correct
		if reflect.TypeOf(fn) == reflect.TypeOf(api.ResourceLimits) {
			rs, err = resourcemetrics.RoleResourceLimits(scaledObject)
			if err != nil {
				return nil, err
			}
		} else {
			rs, err = resourcemetrics.RoleResourceRequests(scaledObject)
			if err != nil {
				return nil, err
			}
		}

		return rs, nil
	}
}
