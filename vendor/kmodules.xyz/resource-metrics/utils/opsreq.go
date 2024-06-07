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

package utils

import (
	"context"
	"errors"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kmapi "kmodules.xyz/client-go/api/v1"
	dbopsapi "kmodules.xyz/resource-metrics/ops.kubedb.com/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// ExpandReferencedAppInOpsObject get the DB resource reference by the OpsRequest
// object and wrap the DB object within it
func ExpandReferencedAppInOpsObject(kc client.Client, u *unstructured.Unstructured) error {
	opsReqObj := u.UnstructuredContent()
	opsPathMapper, err := dbopsapi.LoadOpsPathMapper(u.UnstructuredContent())
	if err != nil {
		return err
	}
	refObjPath := opsPathMapper.GetAppRefPath()
	refObjNamePath := append(refObjPath, "name")

	refObjInfo, err := getOpsRequestReferencedDbObjectInfo(kc, u, refObjNamePath)
	if err != nil {
		return err
	}
	refDb, err := getReferencedDBResource(kc, refObjInfo)
	if err != nil {
		return err
	}
	err = unstructured.SetNestedMap(opsReqObj, refDb.UnstructuredContent(), refObjPath...)
	if err != nil {
		return err
	}
	u.Object = opsReqObj

	return nil
}

// getOpsRequestReferencedDbObjectInfo extracts the referenced database information from OpsRequest object
func getOpsRequestReferencedDbObjectInfo(kc client.Client, u *unstructured.Unstructured, refObjNamePath []string) (*kmapi.ObjectInfo, error) {
	refDbName, ok, err := unstructured.NestedString(u.UnstructuredContent(), refObjNamePath...)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("referenced database name not found")
	}
	ns := u.GetNamespace()

	gvk, err := apiutil.GVKForObject(u, kc.Scheme())
	if err != nil {
		return nil, err
	}

	ri, err := kmapi.ExtractResourceID(kc.RESTMapper(), kmapi.ResourceID{
		Group: strings.TrimPrefix(gvk.Group, "ops."),
		Kind:  strings.TrimSuffix(gvk.Kind, "OpsRequest"),
	})
	if err != nil {
		return nil, err
	}

	return &kmapi.ObjectInfo{
		Resource: *ri,
		Ref: kmapi.ObjectReference{
			Namespace: ns,
			Name:      refDbName,
		},
	}, nil
}

// getReferencedDBResource get the database object referenced by the OpsRequest object and returns it
func getReferencedDBResource(kc client.Client, ri *kmapi.ObjectInfo) (*unstructured.Unstructured, error) {
	dbRes := &unstructured.Unstructured{}
	dbRes.SetGroupVersionKind(ri.Resource.GroupVersionKind())

	err := kc.Get(context.TODO(), types.NamespacedName{Name: ri.Ref.Name, Namespace: ri.Ref.Namespace}, dbRes)
	if err != nil {
		return nil, err
	}

	return dbRes, nil
}
