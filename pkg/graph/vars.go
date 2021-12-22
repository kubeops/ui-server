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

package graph

import (
	"sync"

	ksets "gomodules.xyz/sets/kubernetes"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiv1 "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub"
	setx "kmodules.xyz/resource-metadata/pkg/utils/sets"
)

var reg = hub.NewRegistryOfKnownResources()

var objGraph = &ObjectGraph{
	m:     sync.RWMutex{},
	edges: map[apiv1.OID]map[v1alpha1.EdgeLabel]setx.OID{},
	ids:   map[apiv1.OID]map[v1alpha1.EdgeLabel]setx.OID{},
}

var Schema = getGraphQLSchema()

var resourceChannel = make(chan apiv1.ResourceID, 100)
var resourceTracker = map[schema.GroupVersionKind]apiv1.ResourceID{}

var gkSet = ksets.NewGroupKind(
	schema.GroupKind{
		Group: "admissionregistration.k8s.io",
		Kind:  "ValidatingWebhookConfiguration",
	},
	schema.GroupKind{
		Group: "events.k8s.io",
		Kind:  "Event",
	},
	schema.GroupKind{
		Group: "storage.k8s.io",
		Kind:  "VolumeAttachment",
	},
	schema.GroupKind{
		Group: "admissionregistration.k8s.io",
		Kind:  "MutatingWebhookConfiguration",
	},
	schema.GroupKind{
		Group: "",
		Kind:  "PodTemplate",
	},
	schema.GroupKind{
		Group: "apps",
		Kind:  "ControllerRevision",
	},
	schema.GroupKind{
		Group: "apiextensions.k8s.io",
		Kind:  "CustomResourceDefinition",
	},
	schema.GroupKind{
		Group: "flowcontrol.apiserver.k8s.io",
		Kind:  "PriorityLevelConfiguration",
	},
	schema.GroupKind{
		Group: "",
		Kind:  "Event",
	})
