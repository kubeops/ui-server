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

package resourcemanifests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kubepack.dev/lib-app/pkg/editor"
	"kubepack.dev/lib-helm/pkg/repo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	releasesapi "x-helm.dev/apimachinery/apis/releases/v1alpha1"
)

type Storage struct {
	kc client.Client
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client) *Storage {
	return &Storage{
		kc: kc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindResourceManifests)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rsapi.ResourceKindResourceManifests)
}

// Getter
func (r *Storage) New() runtime.Object {
	return &rsapi.ResourceManifests{}
}

func (r *Storage) Destroy() {}

var DefaultCache = repo.DefaultDiskCache()

func NewRegistry(kc client.Client) repo.IRegistry {
	return repo.NewRegistry(kc, DefaultCache)
}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	manifests, ok := obj.(*rsapi.ResourceManifests)
	if !ok {
		return nil, fmt.Errorf("invalid object type: %T", obj)
	}
	if manifests.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}

	reg := NewRegistry(r.kc)
	metadata := releasesapi.ModelMetadata{Metadata: releasesapi.Metadata{
		Resource: kmapi.ResourceID{
			Group:   manifests.Request.Group,
			Version: manifests.Request.Version,
			Kind:    manifests.Request.Kind,
		},
		Release: releasesapi.ObjectMeta{
			Name:      manifests.Request.Name,
			Namespace: manifests.Request.Namespace,
		},
	}}
	tpl, err := editor.LoadResourceEditorModel(r.kc, reg, metadata)
	if err != nil {
		return nil, err
	}

	mp := make(map[string]runtime.RawExtension)
	for _, resource := range tpl.Resources {
		kmapi.NewObjectID(resource.Data).OID()
		rawJSON, err := json.Marshal(resource.Data)
		if err != nil {
			fmt.Printf("Error marshalling unstructured object: %v\n", err)
			return nil, err
		}
		mp[string(kmapi.NewObjectID(resource.Data).OID())] = runtime.RawExtension{Raw: rawJSON}
	}
	manifests.Response = rsapi.ResourceManifestsResponse{
		Objects: mp,
	}
	return manifests, nil
}
