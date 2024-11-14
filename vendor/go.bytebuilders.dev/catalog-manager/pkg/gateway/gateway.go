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

/*
func GatewayClassName(gwps *catgwapi.GatewayPreset) string {
	return gwps.Name
}

func FrontendTLSRef(gwps *catgwapi.GatewayPreset) types.NamespacedName {
	return types.NamespacedName{
		Namespace: gwps.Namespace,
		Name:      fmt.Sprintf("%s-gw-cert", gwps.Name),
	}
}

func FindGatewayPreset(ctx context.Context, kc client.Client, bindingNamespace string) (*catgwapi.GatewayPreset, error) {
	var gwps catgwapi.GatewayPreset

	key := client.ObjectKey{Name: bindingNamespace, Namespace: bindingNamespace}
	if strings.HasSuffix(bindingNamespace, "-gw") {
		key.Name = strings.TrimSuffix(bindingNamespace, "-gw")

		var ns core.Namespace
		err := kc.Get(ctx, client.ObjectKey{Name: key.Name}, &ns)
		if err != nil {
			return nil, err
		}
		if ns.Labels[kmapi.ClientOrgKey] == "true" {
			if err := kc.Get(ctx, key, &gwps); err == nil {
				return &gwps, nil
			}
		}
	} else {
		key.Namespace += "-gw"

		var ns core.Namespace
		err := kc.Get(ctx, client.ObjectKey{Name: key.Name}, &ns)
		if err != nil {
			return nil, err
		}
		if ns.Labels[kmapi.ClientOrgKey] == "true" {
			if err := kc.Get(ctx, key, &gwps); err == nil {
				return &gwps, nil
			}
		}
	}
	return FindDefaultGatewayPreset(ctx, kc)
}

func FindDefaultGatewayPreset(ctx context.Context, kc client.Client) (*catgwapi.GatewayPreset, error) {
	var list catgwapi.GatewayPresetList
	if err := kc.List(ctx, &list); err != nil {
		return nil, err
	}

	var objs []catgwapi.GatewayPreset
	for _, preset := range list.Items {
		if preset.Annotations[catgwapi.DefaultPresetKey] == "true" {
			objs = append(objs, preset)
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
		return nil, fmt.Errorf("multiple defult gateway presets found: %+v", names)
	}
}

func FindGatewayConfig(ctx context.Context, kc client.Client, bindingNamespace string) (*catgwapi.GatewayPreset, *catgwapi.GatewayConfig, error) {
	gwps, err := FindGatewayPreset(ctx, kc, bindingNamespace)
	if err != nil {
		return nil, nil, err
	}

	gwcfg, err := GetGatewayConfig(ctx, kc, gwps)
	return gwps, gwcfg, err
}

func GetGatewayConfig(ctx context.Context, kc client.Client, gwps *catgwapi.GatewayPreset) (*catgwapi.GatewayConfig, error) {
	if gwps.Spec.ParametersRef == nil {
		return nil, fmt.Errorf("GatewayPreset %s/%s is missing parametersRef", gwps.Namespace, gwps.Name)
	}

	var gwcfg catgwapi.GatewayConfig
	if err := kc.Get(ctx, client.ObjectKey{
		Namespace: gwps.Spec.ParametersRef.Namespace,
		Name:      gwps.Spec.ParametersRef.Name,
	}, &gwcfg); err != nil {
		return nil, err
	}
	return &gwcfg, nil
}

func FindEnvoyServiceSpec(ctx context.Context, kc client.Client, gwclass string) (*catgwapi.EnvoyServiceSpec, error) {
	var ns core.Namespace
	err := kc.Get(ctx, client.ObjectKey{Name: gwclass}, &ns)
	if err != nil {
		return nil, err
	}

	var gwps catgwapi.GatewayPreset
	if ns.Labels[kmapi.ClientOrgKey] == "true" {
		if err := kc.Get(ctx, client.ObjectKey{Name: gwclass, Namespace: gwclass + "-gw"}, &gwps); err != nil {
			return nil, err
		}
	} else {
		if err := kc.Get(ctx, client.ObjectKey{Name: gwclass, Namespace: gwclass}, &gwps); err != nil {
			return nil, err
		}
	}

	gwcfg, err := GetGatewayConfig(ctx, kc, &gwps)
	if err != nil {
		return nil, err
	}
	return &gwcfg.Spec.Envoy.Service, nil
}
*/
