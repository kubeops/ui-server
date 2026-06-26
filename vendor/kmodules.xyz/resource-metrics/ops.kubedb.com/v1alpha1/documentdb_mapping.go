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

import "k8s.io/apimachinery/pkg/runtime/schema"

func init() {
	RegisterOpsPathMapperToPlugins(&DocumentDBOpsRequest{})
}

type DocumentDBOpsRequest struct{}

var _ OpsPathMapper = (*DocumentDBOpsRequest)(nil)

func (m *DocumentDBOpsRequest) HorizontalPathMapping() map[OpsReqPath]ReferencedObjPath {
	return map[OpsReqPath]ReferencedObjPath{
		"spec.horizontalScaling.replicas": "spec.replicas",
	}
}

func (m *DocumentDBOpsRequest) VerticalPathMapping() map[OpsReqPath]ReferencedObjPath {
	return map[OpsReqPath]ReferencedObjPath{
		"spec.verticalScaling.documentdb": "spec.podTemplate.spec.resources",
	}
}

func (m *DocumentDBOpsRequest) VolumeExpansionPathMapping() map[OpsReqPath]ReferencedObjPath {
	return map[OpsReqPath]ReferencedObjPath{
		"spec.volumeExpansion.documentdb": "spec.storage.resources.requests.storage",
		"spec.volumeExpansion.arbiter":    "spec.arbiter.storage.resources.requests.storage",
	}
}

func (m *DocumentDBOpsRequest) GetAppRefPath() []string {
	return []string{"spec", "databaseRef"}
}

func (m *DocumentDBOpsRequest) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "ops.kubedb.com",
		Version: "v1alpha1",
		Kind:    "DocumentDBOpsRequest",
	}
}
