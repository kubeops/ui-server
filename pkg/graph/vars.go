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

	"k8s.io/apimachinery/pkg/runtime/schema"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metadata/hub"
	ksets "kmodules.xyz/sets"
)

var Registry = hub.NewRegistryOfKnownResources()

var objGraph = &ObjectGraph{
	m:     sync.RWMutex{},
	Edges: map[kmapi.OID]map[kmapi.EdgeLabel]map[kmapi.OID]bool{},
	IDs:   map[kmapi.OID]map[kmapi.EdgeLabel]ksets.OID{},
}

var Schema = getGraphQLSchema()

var (
	resourceChannel = make(chan kmapi.ResourceID, 100)
	resourceTracker = map[schema.GroupVersionKind]kmapi.ResourceID{}
)

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
