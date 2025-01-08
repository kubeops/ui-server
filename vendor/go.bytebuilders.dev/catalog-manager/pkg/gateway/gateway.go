/*
Copyright AppsCode Inc. and Contributors

Licensed under the AppsCode Community License 1.0.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://github.com/appscode/licenses/raw/1.0.0/AppsCode-Community-1.0.0.md

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gateway

import (
	"context"
	"fmt"
	"strings"

	catgwapi "go.bytebuilders.dev/catalog/api/gateway/v1alpha1"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"
)

func FindDefaultGatewayClass(ctx context.Context, kc client.Client) (*gwv1.GatewayClass, error) {
	var list gwv1.GatewayClassList
	if err := kc.List(ctx, &list); err != nil {
		return nil, err
	}

	var objs []gwv1.GatewayClass
	for _, gwc := range list.Items {
		if gwc.Annotations[catgwapi.DefaultGatewayClassKey] == "true" {
			objs = append(objs, gwc)
		}
	}
	switch len(objs) {
	case 0:
		return nil, nil
	case 1:
		return &objs[0], nil
	default:
		var names []string
		for _, obj := range objs {
			names = append(names, client.ObjectKeyFromObject(&obj).String())
		}
		return nil, fmt.Errorf("multiple defult gateway classes found: %+v", names)
	}
}

func FindGatewayClass(ctx context.Context, kc client.Client, bindingNamespace string) (*gwv1.GatewayClass, error) {
	key := client.ObjectKey{Name: bindingNamespace}
	if strings.HasSuffix(bindingNamespace, "-gw") {
		key.Name = strings.TrimSuffix(bindingNamespace, "-gw")
	}

	var ns core.Namespace
	err := kc.Get(ctx, key, &ns)
	if err != nil {
		return nil, err
	}
	if ns.Labels[kmapi.ClientOrgKey] == "true" {
		var qwc gwv1.GatewayClass
		if err := kc.Get(ctx, key, &qwc); err == nil {
			return &qwc, nil
		}
	}

	return FindDefaultGatewayClass(ctx, kc)
}

func GetGatewayParameter(kc client.Client, gwc *gwv1.GatewayClass) (*catgwapi.GatewayParameter, error) {
	v, ok := gwc.Annotations[catgwapi.GatewayConfigKey]
	if !ok {
		return nil, nil
	}
	var gwp catgwapi.GatewayParameter
	err := yaml.Unmarshal([]byte(v), &gwp)
	if err != nil {
		return nil, err
	}
	if gwp.Service.PortRange == "" {
		gwp.Service.PortRange = catgwapi.DefaultPortRange
	}
	if gwp.Service.NodeportRange == "" {
		gwp.Service.NodeportRange = catgwapi.DefaultNodeportRange
	}
	gwp.GatewayClassName = gwc.Name
	gwp.ServiceType, err = GetGatewayServiceType(context.TODO(), kc, gwc)
	if err != nil {
		return nil, err
	}
	return &gwp, nil
}

func FindGatewayParameter(ctx context.Context, kc client.Client, bindingNamespace string) (*catgwapi.GatewayParameter, error) {
	gwc, err := FindGatewayClass(ctx, kc, bindingNamespace)
	if err != nil {
		return nil, err
	}

	return GetGatewayParameter(kc, gwc)
}

func GatewayName(db metav1.Object) string {
	return db.GetName()
}

func FindDefaultGatewayConfig(ctx context.Context, kc client.Client) (*catgwapi.GatewayConfig, error) {
	var list catgwapi.GatewayConfigList
	if err := kc.List(ctx, &list); err != nil {
		return nil, err
	}

	var objs []catgwapi.GatewayConfig
	for _, cfg := range list.Items {
		if cfg.Annotations[catgwapi.DefaultConfigKey] == "true" {
			objs = append(objs, cfg)
		}
	}
	switch len(objs) {
	case 0:
		return nil, nil
	case 1:
		return &objs[0], nil
	default:
		var names []string
		for _, obj := range objs {
			names = append(names, client.ObjectKeyFromObject(&obj).String())
		}
		return nil, fmt.Errorf("multiple defult gateway configs found: %+v", names)
	}
}
