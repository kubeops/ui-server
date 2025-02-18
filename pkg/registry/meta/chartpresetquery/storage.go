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

package chartpresetquery

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/zeebo/xxh3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kubepack.dev/lib-helm/pkg/values"
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
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client) *Storage {
	return &Storage{
		kc: kc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindChartPresetQuery)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rsapi.ResourceKindChartPresetQuery)
}

func (r *Storage) New() runtime.Object {
	return &rsapi.ChartPresetQuery{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*rsapi.ChartPresetQuery)
	if in.Request == nil {
		return nil, apierrors.NewBadRequest("missing apirequest")
	}

	presets, err := values.LoadPresetValues(r.kc, *in.Request)
	if err != nil {
		return nil, err
	}

	var state string
	oids := make([]string, 0, len(presets))
	for _, preset := range presets {
		if preset.Source.Resource.Group == "" ||
			preset.Source.Resource.Kind == "" ||
			preset.Source.UID == "" ||
			preset.Source.Generation <= 0 {
			break
		}
		oids = append(oids, fmt.Sprintf("G=%s,K=%s,I=%s,V=%d",
			preset.Source.Resource.Group,
			preset.Source.Resource.Kind,
			preset.Source.UID,
			preset.Source.Generation,
		))
	}
	// only calculate state hash if all presets have necessary data
	if len(oids) == len(presets) {
		sort.Strings(oids)
		h := xxh3.New()
		for _, oid := range oids {
			_, _ = h.WriteString(oid)
			_, _ = h.WriteString(";")
		}
		state = strconv.FormatUint(h.Sum64(), 10)
	}

	in.Response = &rsapi.ChartPresetQueryResponse{
		Presets: presets,
		State:   state,
	}

	return in, nil
}
