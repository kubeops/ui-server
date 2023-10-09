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

package resourceCalculator

import (
	"context"
	"errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

// ReferencedObjInfo indicates the information about the referenced database
// object by the kubedb OpsRequest object
type ReferencedObjInfo struct {
	group     string
	version   string
	kind      string
	name      string
	namespace string
}

const (
	DBGroup   = "kubedb.com"
	DBVersion = "v1alpha2"
)

// wrapReferencedDBResourceWithOpsReqObject get the DB resource reference by the OpsRequest
// object and wrap the DB object within it as 'referencedDB'
func wrapReferencedDBResourceWithOpsReqObject(kc client.Client, u *unstructured.Unstructured) error {
	opsReqObj := u.UnstructuredContent()
	defer func() {
		u.Object = opsReqObj
	}()
	// Get the referenced db info
	refObjInfo, err := getOpsRequestReferencedDbObjectInfo(u)
	if err != nil {
		return err
	}
	// Get the referenced db object
	refDb, err := getReferencedDBResource(kc, refObjInfo)
	if err != nil {
		return err
	}
	// Wrap the referenced db object with the OpsRequest object
	err = unstructured.SetNestedMap(opsReqObj, refDb.UnstructuredContent(), "spec", "databaseRef", "referencedDB")
	if err != nil {
		return err
	}

	return nil
}

// getOpsRequestReferencedDbObjectInfo extracts the referenced database information from OpsRequest object
func getOpsRequestReferencedDbObjectInfo(u *unstructured.Unstructured) (*ReferencedObjInfo, error) {
	refDbName, ok, err := unstructured.NestedString(u.UnstructuredContent(), "spec", "databaseRef", "name")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("referenced database name not found")
	}
	ns := u.GetNamespace()
	kind := strings.TrimSuffix(u.GetKind(), "OpsRequest")

	return &ReferencedObjInfo{
		group:     DBGroup,
		version:   DBVersion,
		kind:      kind,
		name:      refDbName,
		namespace: ns,
	}, nil
}

// getReferencedDBResource get the database object referenced by the OpsRequest object and returns it
func getReferencedDBResource(kc client.Client, ri *ReferencedObjInfo) (*unstructured.Unstructured, error) {
	dbRes := &unstructured.Unstructured{}
	dbRes.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   ri.group,
		Version: ri.version,
		Kind:    ri.kind,
	})

	err := kc.Get(context.TODO(), types.NamespacedName{Name: ri.name, Namespace: ri.namespace}, dbRes)
	if err != nil {
		return nil, err
	}

	return dbRes, nil
}
