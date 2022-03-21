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

package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	"go.openviz.dev/apimachinery/crds"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/client-go/apiextensions"
	appcatalogapi "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (_ GrafanaDashboard) CustomResourceDefinition() *apiextensions.CustomResourceDefinition {
	return crds.MustCustomResourceDefinition(SchemeGroupVersion.WithResource(ResourceGrafanaDashboards))
}

func GetGrafana(ctx context.Context, kc client.Client, ref *kmapi.ObjectReference) (*appcatalogapi.AppBinding, error) {
	if ref != nil {
		var grafana appcatalogapi.AppBinding
		err := kc.Get(ctx, ref.ObjectKey(), &grafana)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch AppBinding %s", ref.ObjectKey())
		}
		return &grafana, nil
	}

	var grafanaList appcatalogapi.AppBindingList
	// any namespace, default Grafana AppBinding
	if err := kc.List(ctx, &grafanaList, client.MatchingFields{
		mona.DefaultGrafanaKey: "true",
	}); err != nil {
		return nil, err
	}
	if len(grafanaList.Items) == 0 {
		return nil, apierrors.NewNotFound(appcatalogapi.Resource(appcatalogapi.ResourceKindApp), "no default Grafana appbinding found.")
	} else if len(grafanaList.Items) > 1 {
		names := make([]string, len(grafanaList.Items))
		for idx, item := range grafanaList.Items {
			names[idx] = fmt.Sprintf("%s/%s", item.Namespace, item.Name)
		}
		return nil, apierrors.NewBadRequest(fmt.Sprintf("multiple Grafana appbindings %s are marked default", strings.Join(names, ",")))
	}
	return &grafanaList.Items[0], nil
}
