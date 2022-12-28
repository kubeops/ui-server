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

package image

import (
	"context"
	"sort"

	reportsapi "kubeops.dev/scanner/apis/reports/v1alpha1"
	"kubeops.dev/ui-server/pkg/graph"
	"kubeops.dev/ui-server/pkg/shared"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/client-go/client/apiutil"
	"kmodules.xyz/go-containerregistry/name"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc client.Client
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
)

func NewStorage(kc client.Client) *Storage {
	return &Storage{
		kc: kc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return reportsapi.SchemeGroupVersion.WithKind(reportsapi.ResourceKindImage)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &reportsapi.Image{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*reportsapi.Image)

	var oi *kmapi.ObjectInfo
	if in.Request != nil {
		oi = &in.Request.ObjectInfo
	}
	pods, err := graph.LocatePods(ctx, r.kc, oi)
	if err != nil {
		return nil, err
	}

	images := map[string]kmapi.ImageInfo{}
	for _, p := range pods {
		var pod core.Pod
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(p.UnstructuredContent(), &pod); err != nil {
			return nil, err
		}
		images, err = apiutil.CollectImageInfo(r.kc, &pod, images, true)
		if err != nil {
			return nil, err
		}
	}
	// For image, keep ImageInfo if found in any pods or just try as a image name
	if shared.IsImageRequest(oi) {
		ref, err := name.ParseReference(in.Request.Ref.Name)
		if err != nil {
			return nil, err
		}
		if info, ok := images[ref.Name]; ok {
			images = map[string]kmapi.ImageInfo{
				ref.Name: info,
			}
		} else {
			images = map[string]kmapi.ImageInfo{
				ref.Name: {
					Image: ref.Name,
				},
			}
		}
	}

	out := make([]kmapi.ImageInfo, 0, len(images))
	for _, info := range images {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Image < out[j].Image
	})

	in.Response = &reportsapi.ImageResponse{
		Images: out,
	}
	return in, nil
}
