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

package v1alpha2

import (
	"fmt"

	"kmodules.xyz/resource-metrics/api"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func init() {
	api.Register(schema.GroupVersionKind{
		Group:   "kubedb.com",
		Version: "v1alpha2",
		Kind:    "Milvus",
	}, Milvus{}.ResourceCalculator())
}

type Milvus struct{}

func (m Milvus) ResourceCalculator() api.ResourceCalculator {
	return &api.ResourceCalculatorFuncs{
		AppRoles:               []api.PodRole{api.PodRoleDefault, api.PodRoleDataNode, api.PodRoleMixCoord, api.PodRoleQueryNode, api.PodRoleStreamingNode, api.PodRoleProxy},
		RuntimeRoles:           []api.PodRole{api.PodRoleDefault, api.PodRoleDataNode, api.PodRoleMixCoord, api.PodRoleQueryNode, api.PodRoleStreamingNode, api.PodRoleProxy},
		RoleReplicasFn:         m.roleReplicasFn,
		ModeFn:                 m.modeFn,
		UsesTLSFn:              m.usesTLSFn,
		RoleResourceLimitsFn:   m.roleResourceFn(api.ResourceLimits),
		RoleResourceRequestsFn: m.roleResourceFn(api.ResourceRequests),
	}
}

func (m Milvus) usesTLSFn(obj map[string]any) (bool, error) {
	_, found, err := unstructured.NestedFieldNoCopy(obj, "spec", "tls")
	return found, err
}

func (m Milvus) roleReplicasFn(obj map[string]any) (api.ReplicaList, error) {
	result := api.ReplicaList{}

	mode, err := m.modeFn(obj)
	if err != nil {
		return nil, err
	}

	if mode == DBModeDistributed {
		distributed, found, err := unstructured.NestedMap(obj, "spec", "topology", "distributed")
		if err != nil {
			return nil, err
		}

		if found && distributed != nil {
			for role, roleSpec := range distributed {
				roleSpecMap, ok := roleSpec.(map[string]any)
				if !ok {
					continue
				}
				roleReplicas, found, err := unstructured.NestedInt64(roleSpecMap, "replicas")
				if err != nil {
					return nil, err
				}
				if found {
					result[api.PodRole(role)] = roleReplicas
				}
			}
		}
	} else {
		replicas, found, err := unstructured.NestedInt64(obj, "spec", "replicas")
		if err != nil {
			return nil, fmt.Errorf("failed to read spec.replicas %v: %w", obj, err)
		}
		if !found {
			result[api.PodRoleDefault] = 1
		} else {
			result[api.PodRoleDefault] = replicas
		}
	}

	return result, nil
}

func (m Milvus) modeFn(obj map[string]any) (string, error) {
	mode, found, err := unstructured.NestedString(obj, "spec", "topology", "mode")
	if err != nil {
		return "", err
	}
	if found && mode == "Distributed" {
		return DBModeDistributed, nil
	}
	return DBModeStandalone, nil
}

func (r Milvus) roleResourceFn(fn func(rr core.ResourceRequirements) core.ResourceList) func(obj map[string]any) (map[api.PodRole]api.PodInfo, error) {
	return func(obj map[string]any) (map[api.PodRole]api.PodInfo, error) {
		mode, err := r.modeFn(obj)
		if err != nil {
			return nil, err
		}
		result := map[api.PodRole]api.PodInfo{}
		if mode == DBModeDistributed {
			distributed, found, err := unstructured.NestedMap(obj, "spec", "topology", "distributed")
			if err != nil {
				return nil, err
			}
			if found && distributed != nil {
				var replicas int64 = 0
				for role, roleSpec := range distributed {
					roleSpecMap, ok := roleSpec.(map[string]any)
					if !ok {
						continue
					}
					rolePerReplicaResources, roleReplicas, err := api.AppNodeResourcesV2(roleSpecMap, fn, MilvusContainerName)
					if err != nil {
						return nil, err
					}
					result[api.PodRole(role)] = api.PodInfo{
						Resource: rolePerReplicaResources,
						Replicas: roleReplicas,
					}
					replicas += roleReplicas
				}

				result[api.PodRoleDefault] = api.PodInfo{
					Resource: nil,
					Replicas: replicas,
				}
				return result, nil
			}
		}

		container, replicas, err := api.AppNodeResourcesV2(obj, fn, MilvusContainerName, "spec")
		if err != nil {
			return nil, err
		}
		result[api.PodRoleDefault] = api.PodInfo{
			Resource: container,
			Replicas: replicas,
		}
		return result, nil
	}
}
