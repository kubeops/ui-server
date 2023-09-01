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

package reports

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	costapi "kubeops.dev/ui-server/apis/cost/v1alpha1"

	gs "github.com/gorilla/schema"
	"github.com/pkg/errors"
	"gomodules.xyz/sync"
	core "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	once  sync.Once
	ocURL *url.URL
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
	return costapi.SchemeGroupVersion.WithKind(costapi.ResourceKindCostReport)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &costapi.CostReport{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*costapi.CostReport)

	once.Do(func() error {
		var svcs core.ServiceList
		err := r.kc.List(ctx, &svcs, client.MatchingLabels{
			"app.kubernetes.io/name": "opencost",
		})
		if err != nil {
			return err
		} else if len(svcs.Items) == 0 {
			return errors.New("opencost service not found")
		}

		ocSvc := svcs.Items[0]
		var ocApiPort int32
		for _, p := range ocSvc.Spec.Ports {
			if p.Name == "http" {
				ocApiPort = p.Port
				break
			}
		}
		if ocApiPort <= 0 {
			return errors.New("missing http port in opencost service spec")
		}

		ocURL, err = url.Parse(fmt.Sprintf("http://%s.%s.svc:%d/model/allocation/compute", ocSvc.Name, ocSvc.Namespace, ocApiPort))
		return err
	})

	encoder := gs.NewEncoder()
	form := url.Values{}
	err := encoder.Encode(in.Request, form)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode cost report request")
	}

	u := *ocURL
	u.RawQuery = form.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create opencost service request")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call opencost service")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read opencost api response")
	}
	in.Response = &apiextensionsv1.JSON{Raw: body}
	return in, nil
}
