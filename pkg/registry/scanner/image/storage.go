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
	"fmt"
	"sort"
	"strings"

	reportsapi "kubeops.dev/scanner/apis/reports/v1alpha1"
	"kubeops.dev/ui-server/pkg/graph"

	"github.com/google/go-containerregistry/pkg/name"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	sharedapi "kmodules.xyz/resource-metadata/apis/shared"
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
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}

	rid := in.Request.Resource

	var pods []core.Pod
	if rid.Group == "" && (rid.Kind == "Pod" || rid.Name == "pods") {
		var pod core.Pod
		if err := r.kc.Get(ctx, in.Request.Ref.ObjectKey(), &pod); err != nil {
			return nil, err
		}
		pods = append(pods, pod)
	} else {
		if rid.Kind == "" {
			r2, err := kmapi.ExtractResourceID(r.kc.RESTMapper(), in.Request.Resource)
			if err != nil {
				return nil, err
			}
			rid = *r2
		}

		src := kmapi.ObjectID{
			Group:     rid.Group,
			Kind:      rid.Kind,
			Namespace: in.Request.Ref.Namespace,
			Name:      in.Request.Ref.Name,
		}
		target := sharedapi.ResourceLocator{
			Ref: metav1.GroupKind{
				Group: "",
				Kind:  "Pod",
			},
			Query: sharedapi.ResourceQuery{
				Type:    sharedapi.GraphQLQuery,
				ByLabel: kmapi.EdgeOffshoot,
			},
		}

		_, refs, err := graph.ExecRawQuery(r.kc, src.OID(), target)
		if err != nil {
			return nil, err
		}

		pods = make([]core.Pod, 0, len(refs))
		for _, ref := range refs {
			var pod core.Pod
			if err := r.kc.Get(ctx, ref.ObjectKey(), &pod); err != nil {
				return nil, err
			}
			pods = append(pods, pod)
		}
	}

	type info struct {
		containers sets.String
		pods       sets.String
	}
	stats := map[string]info{}

	for _, pod := range pods {
		fn := func(pod string, containers []core.Container, c core.ContainerStatus) error {
			var img string
			if strings.ContainsRune(c.Image, '@') {
				img = c.Image
			} else if strings.HasPrefix(c.ImageID, "sha256:") {
				for _, container := range containers {
					if container.Name == c.Name {
						img = container.Image
						break
					}
				}
			} else if strings.HasPrefix(c.ImageID, "docker-pullable://") {
				img = c.ImageID[len("docker-pullable://"):] // Linode
			} else {
				_, digest, ok := strings.Cut(c.ImageID, "@")
				if !ok {
					return fmt.Errorf("missing digest in pod %s container %s imageID %s", pod, c.Name, c.ImageID)
				}
				img = c.Image + "@" + digest
			}
			ref, err := name.ParseReference(img)
			if err != nil {
				return err
			}
			processed := ref.Name()
			inf, ok := stats[processed]
			if !ok {
				inf = info{
					containers: sets.NewString(),
					pods:       sets.NewString(),
				}
				stats[processed] = inf
			}
			inf.containers.Insert(c.Name)
			inf.pods.Insert(pod)
			return nil
		}

		for _, c := range pod.Status.ContainerStatuses {
			if err := fn(pod.Name, pod.Spec.Containers, c); err != nil {
				return nil, err
			}
		}
		for _, c := range pod.Status.InitContainerStatuses {
			if err := fn(pod.Name, pod.Spec.InitContainers, c); err != nil {
				return nil, err
			}
		}
	}

	in.Response = &reportsapi.ImageResponse{
		Images: make([]reportsapi.ImageInfo, 0, len(stats)),
	}
	for img, info := range stats {
		in.Response.Images = append(in.Response.Images, reportsapi.ImageInfo{
			Image:      img,
			Containers: info.containers.List(),
			Pods:       info.pods.List(),
		})
	}
	sort.Slice(in.Response.Images, func(i, j int) bool {
		return in.Response.Images[i].Image < in.Response.Images[j].Image
	})
	return in, nil
}
