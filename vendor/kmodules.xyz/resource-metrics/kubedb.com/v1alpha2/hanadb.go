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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func init() {
	api.Register(schema.GroupVersionKind{
		Group:   "kubedb.com",
		Version: "v1alpha2",
		Kind:    "HanaDB",
	}, HanaDB{}.ResourceCalculator())
}

type HanaDB struct{}

var (
	hanaDBCoordinatorDefaultResources = core.ResourceRequirements{
		Requests: core.ResourceList{
			core.ResourceCPU:    resource.MustParse(".200"),
			core.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: core.ResourceList{
			core.ResourceMemory: resource.MustParse("256Mi"),
		},
	}
	hanaDBArbiterDefaultResources = core.ResourceRequirements{
		Requests: core.ResourceList{
			core.ResourceStorage: resource.MustParse("2Gi"),
			core.ResourceCPU:     resource.MustParse(".200"),
			core.ResourceMemory:  resource.MustParse("256Mi"),
		},
		Limits: core.ResourceList{
			core.ResourceMemory: resource.MustParse("256Mi"),
		},
	}
)

func (r HanaDB) ResourceCalculator() api.ResourceCalculator {
	return &api.ResourceCalculatorFuncs{
		AppRoles:               []api.PodRole{api.PodRoleDefault},
		RuntimeRoles:           []api.PodRole{api.PodRoleDefault, api.PodRoleCoordinator, api.PodRoleExporter, api.PodRoleArbiter},
		RoleReplicasFn:         r.roleReplicasFn,
		ModeFn:                 r.modeFn,
		RoleResourceLimitsFn:   r.roleResourceFn(api.ResourceLimits),
		RoleResourceRequestsFn: r.roleResourceFn(api.ResourceRequests),
	}
}

func (r HanaDB) roleReplicasFn(obj map[string]any) (api.ReplicaList, error) {
	replicas, found, err := unstructured.NestedInt64(obj, "spec", "replicas")
	if err != nil {
		return nil, fmt.Errorf("failed to read spec.replicas %v: %w", obj, err)
	}
	if !found {
		replicas = 1
	}

	result := api.ReplicaList{
		api.PodRoleDefault: replicas,
	}
	if replicas > 1 {
		result[api.PodRoleCoordinator] = replicas
		if replicas%2 == 0 {
			result[api.PodRoleArbiter] = 1
		}
	}
	return result, nil
}

func (r HanaDB) modeFn(obj map[string]any) (string, error) {
	mode, found, err := unstructured.NestedString(obj, "spec", "topology", "mode")
	if err != nil {
		return "", fmt.Errorf("failed to read spec.topology.mode %v: %w", obj, err)
	}
	if found {
		return mode, nil
	}
	return DBModeStandalone, nil
}

func (r HanaDB) roleResourceFn(fn func(rr core.ResourceRequirements) core.ResourceList) func(obj map[string]any) (map[api.PodRole]api.PodInfo, error) {
	return func(obj map[string]any) (map[api.PodRole]api.PodInfo, error) {
		container, replicas, err := api.AppNodeResourcesV2(obj, fn, HanaDBContainerName, "spec")
		if err != nil {
			return nil, err
		}

		result := map[api.PodRole]api.PodInfo{
			api.PodRoleDefault: {Resource: container, Replicas: replicas},
		}

		exporter, err := api.ContainerResources(obj, fn, "spec", "monitor", "prometheus", "exporter")
		if err != nil {
			return nil, err
		}
		result[api.PodRoleExporter] = api.PodInfo{Resource: exporter, Replicas: replicas}

		if replicas > 1 {
			coordinator, err := api.SidecarNodeResourcesV2(obj, fn, HanaDBCoordinatorContainerName, "spec")
			if err != nil {
				coordinator = fn(hanaDBCoordinatorDefaultResources)
			}
			result[api.PodRoleCoordinator] = api.PodInfo{Resource: coordinator, Replicas: replicas}
		}

		if replicas%2 == 0 {
			arbiterObj, found, err := unstructured.NestedMap(obj, "spec", "arbiter")
			if err != nil {
				return nil, fmt.Errorf("failed to read spec.arbiter %v: %w", obj, err)
			}
			if found {
				var arbiter struct {
					Resources core.ResourceRequirements `json:"resources,omitempty"`
				}
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(arbiterObj, &arbiter); err != nil {
					return nil, fmt.Errorf("failed to parse arbiter resources %#v: %w", arbiterObj, err)
				}
				resources := arbiter.Resources
				if len(resources.Requests) == 0 && len(resources.Limits) == 0 {
					resources = hanaDBArbiterDefaultResources
				}
				result[api.PodRoleArbiter] = api.PodInfo{Resource: fn(resources), Replicas: 1}
			} else {
				result[api.PodRoleArbiter] = api.PodInfo{Resource: fn(hanaDBArbiterDefaultResources), Replicas: 1}
			}
		}

		return result, nil
	}
}
