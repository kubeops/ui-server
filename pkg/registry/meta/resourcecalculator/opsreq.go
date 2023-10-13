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
// object by the OpsRequest object
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
	OpsReqObj := u.UnstructuredContent()
	defer func() {
		u.Object = OpsReqObj
	}()

	// Get the referenced database info
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
	OpsReqObj["referencedDB"] = refDb

	return nil
}

// getOpsRequestReferencedDbObjectInfo extracts the referenced database information from OpsRequest object
func getOpsRequestReferencedDbObjectInfo(u *unstructured.Unstructured) (*ReferencedObjInfo, error) {
	refDBName, ok, err := unstructured.NestedString(u.UnstructuredContent(), "spec", "databaseRef")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("referenced database name not found")
	}
	kind := strings.TrimSuffix(u.GetKind(), "OpsRequest")
	ns := u.GetNamespace()

	return &ReferencedObjInfo{
		group:     DBGroup,
		version:   DBVersion,
		kind:      kind,
		name:      refDBName,
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
