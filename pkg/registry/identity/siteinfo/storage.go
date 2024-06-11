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

package siteinfo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gomodules.xyz/sync"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	identityapi "kmodules.xyz/resource-metadata/apis/identity/v1alpha1"
	identitylib "kmodules.xyz/resource-metadata/pkg/identity"
	"kmodules.xyz/resource-metrics/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	cfg       *restclient.Config
	kc        kubernetes.Interface
	rtc       client.Client
	convertor rest.TableConvertor

	si   *identityapi.SiteInfo
	once sync.Once
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Lister                   = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(cfg *restclient.Config, kc kubernetes.Interface, rtc client.Client) *Storage {
	return &Storage{
		cfg: cfg,
		kc:  kc,
		rtc: rtc,
		convertor: rest.NewDefaultTableConvertor(schema.GroupResource{
			Group:    identityapi.GroupName,
			Resource: identityapi.ResourceSiteInfos,
		}),
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return identityapi.SchemeGroupVersion.WithKind(identityapi.ResourceKindSiteInfo)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(identityapi.ResourceKindSiteInfo)
}

func (r *Storage) New() runtime.Object {
	return &identityapi.SiteInfo{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	if name != identitylib.SelfName {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: identityapi.GroupName, Resource: identityapi.ResourceSiteInfos}, name)
	}

	return r.getCurrentSiteInfo()
}

// Lister
func (r *Storage) NewList() runtime.Object {
	return &identityapi.SiteInfoList{}
}

func (r *Storage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	si, err := r.getCurrentSiteInfo()
	if err != nil {
		return nil, err
	}
	result := identityapi.SiteInfoList{
		TypeMeta: metav1.TypeMeta{},
		Items: []identityapi.SiteInfo{
			*si,
		},
	}
	return &result, nil
}

func (r *Storage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.convertor.ConvertToTable(ctx, object, tableOptions)
}

func (r *Storage) getCurrentSiteInfo() (*identityapi.SiteInfo, error) {
	r.once.Do(func() error {
		si, err := identitylib.GetSiteInfo(r.cfg, r.kc, nil, "")
		if err != nil {
			return err
		}
		si.Product = nil
		si.ObjectMeta = metav1.ObjectMeta{
			Name:              identitylib.SelfName,
			UID:               types.UID(uuid.Must(uuid.NewUUID()).String()),
			CreationTimestamp: metav1.Time{Time: time.Now()},
		}
		r.si = si
		return nil
	})
	if r.si == nil {
		return nil, errors.New("unable to init site info")
	}

	si := r.si.DeepCopy()

	var nodes core.NodeList
	err := r.rtc.List(context.TODO(), &nodes)
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
