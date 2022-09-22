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

package SiteInfo

import (
	"context"
	"time"

	"github.com/google/uuid"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	auditorv1alpha1 "kmodules.xyz/custom-resources/apis/auditor/v1alpha1"
	su "kmodules.xyz/custom-resources/util/siteinfo"
	"kmodules.xyz/resource-metrics/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	c  client.Client
	si *auditorv1alpha1.SiteInfo
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
)

func NewStorage(cfg *restclient.Config, c client.Client) (*Storage, error) {
	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	si, err := su.GetSiteInfo(cfg, kc, nil, "")
	if err != nil {
		return nil, err
	}
	si.Product = nil
	si.ObjectMeta = metav1.ObjectMeta{
		Name:              "default",
		UID:               types.UID(uuid.Must(uuid.NewUUID()).String()),
		CreationTimestamp: metav1.Time{Time: time.Now()},
	}

	return &Storage{
		c:  c,
		si: si,
	}, nil
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return auditorv1alpha1.SchemeGroupVersion.WithKind(auditorv1alpha1.ResourceKindSiteInfo)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &auditorv1alpha1.SiteInfo{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, _ runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	si := r.si.DeepCopy()

	var nodes core.NodeList
	err := r.c.List(ctx, &nodes)
	if err != nil {
		return nil, err
	}

	si.Kubernetes.NodeStats.Count = len(nodes.Items)

	var capacity core.ResourceList
	var allocatable core.ResourceList
	for _, node := range nodes.Items {
		capacity = api.AddResourceList(capacity, node.Status.Capacity)
		allocatable = api.AddResourceList(allocatable, node.Status.Allocatable)
	}
	si.Kubernetes.NodeStats.Capacity = capacity
	si.Kubernetes.NodeStats.Allocatable = allocatable

	return si, nil
}
