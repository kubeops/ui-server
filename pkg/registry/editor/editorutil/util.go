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

package editorutil

import (
	"context"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/rest"
	meta_util "kmodules.xyz/client-go/meta"
	"kubepack.dev/lib-helm/pkg/repo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	releasesapi "x-helm.dev/apimachinery/apis/releases/v1alpha1"
)

// defaultCache is the chart disk cache shared across requests.
var defaultCache = repo.DefaultDiskCache()

// CallerClient returns a controller-runtime client and a lib-helm chart registry
// that impersonate the API caller, so every read performed while rendering or
// loading an editor is authorized against the caller's own RBAC.
func CallerClient(ctx context.Context, cfg *rest.Config, scheme *runtime.Scheme, mapper meta.RESTMapper) (client.Client, repo.IRegistry, error) {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, nil, apierrors.NewBadRequest("missing user info in request context")
	}

	impCfg := rest.CopyConfig(cfg)
	impCfg.Impersonate = rest.ImpersonationConfig{
		UserName: user.GetName(),
		UID:      user.GetUID(),
		Groups:   user.GetGroups(),
		Extra:    user.GetExtra(),
	}

	kc, err := client.New(impCfg, client.Options{Scheme: scheme, Mapper: mapper})
	if err != nil {
		return nil, nil, err
	}
	return kc, repo.NewRegistry(kc, defaultCache), nil
}

// RenderedResourceOutput converts a rendered chart template's CRDs and resources
// into a releasesapi.ResourceOutput, mirroring b3 deploy.PreviewEditorResources:
// CRDs and resources are marshaled from their object Data, and resources are
// sorted so kubedb.com objects come first.
func RenderedResourceOutput(crds []releasesapi.BucketObject, resources []releasesapi.ResourceObject, skipCRDs bool, format meta_util.DataFormat) (*releasesapi.ResourceOutput, error) {
	var out releasesapi.ResourceOutput

	if !skipCRDs {
		for _, crd := range crds {
			data, err := meta_util.Marshal(crd.Data, format)
			if err != nil {
				return nil, err
			}
			out.CRDs = append(out.CRDs, releasesapi.ResourceFile{
				Filename: crd.Filename + "." + string(format),
				Data:     string(data),
			})
		}
	}

	sortKubeDBFirst(resources)

	for _, r := range resources {
		data, err := meta_util.Marshal(r.Data, format)
		if err != nil {
			return nil, err
		}
		out.Resources = append(out.Resources, releasesapi.ResourceFile{
			Filename: r.Filename + "." + string(format),
			Key:      r.Key,
			Data:     string(data),
		})
	}
	return &out, nil
}

// LoadedResourceOutput converts the resources of a loaded editor template into a
// releasesapi.ResourceOutput, mirroring b3 deploy.LoadEditorResources (each
// ResourceObject is marshaled whole, no CRDs section, no reordering).
func LoadedResourceOutput(resources []releasesapi.ResourceObject, format meta_util.DataFormat) (*releasesapi.ResourceOutput, error) {
	var out releasesapi.ResourceOutput
	for _, r := range resources {
		data, err := meta_util.Marshal(r, format)
		if err != nil {
			return nil, err
		}
		out.Resources = append(out.Resources, releasesapi.ResourceFile{
			Filename: r.Filename + "." + string(format),
			Key:      r.Key,
			Data:     string(data),
		})
	}
	return &out, nil
}

func sortKubeDBFirst(resources []releasesapi.ResourceObject) {
	hasKubeDB := func(r releasesapi.ResourceObject) bool {
		if r.Data == nil {
			return false
		}
		apiVersion, found, err := unstructured.NestedString(r.Data.Object, "apiVersion")
		if err != nil || !found {
			return false
		}
		parts := strings.SplitN(apiVersion, "/", 2)
		if len(parts) < 1 {
			return false
		}
		return parts[0] == "kubedb.com"
	}
	sort.SliceStable(resources, func(i, j int) bool {
		iIsKubeDB := hasKubeDB(resources[i])
		jIsKubeDB := hasKubeDB(resources[j])
		if iIsKubeDB && !jIsKubeDB {
			return true
		}
		if !iIsKubeDB && jIsKubeDB {
			return false
		}
		return i < j
	})
}
