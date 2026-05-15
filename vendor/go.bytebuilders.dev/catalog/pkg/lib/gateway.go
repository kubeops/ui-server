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

package lib

import (
	"context"
	"fmt"
	"strings"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	catgwapi "go.bytebuilders.dev/catalog/api/gateway/v1alpha1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	kmapi "kmodules.xyz/client-go/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"
)

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
		klog.Infof("Finding gatewayClass for client-org %s \n", bindingNamespace)
		var qwc gwv1.GatewayClass
		if err := kc.Get(ctx, key, &qwc); err == nil {
			klog.Infof("Found gateway class %s directly", qwc.Name)
			return &qwc, nil
		}
		class, err := findGWClassFromPresetsRef(ctx, kc, key)
		if err != nil {
			return nil, err
		}
		if class != nil {
			klog.Infof("Found gateway class %s from presetsRef", class.Name)
			return class, nil
		}
	}

	klog.Infof("Finding default gateway class for binding namespace: %s \n", bindingNamespace)
	return FindDefaultGatewayClass(ctx, kc)
}

func findGWClassFromPresetsRef(ctx context.Context, kc client.Client, key client.ObjectKey) (*gwv1.GatewayClass, error) {
	var gwps catgwapi.GatewayPreset
	err := kc.Get(ctx, types.NamespacedName{
		Namespace: key.Name + "-gw",
		Name:      key.Name,
	}, &gwps)
	if err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	if gwps.Spec.ParametersRef != nil {
		var gwcfg catgwapi.GatewayConfig
		err = kc.Get(ctx, types.NamespacedName{
			Namespace: string(*gwps.Spec.ParametersRef.Namespace),
			Name:      gwps.Spec.ParametersRef.Name,
		}, &gwcfg)
		if err != nil {
			return nil, client.IgnoreNotFound(err)
		}

		var qwc gwv1.GatewayClass
		if err := kc.Get(ctx, types.NamespacedName{Name: gwcfg.Name}, &qwc); err == nil {
			return &qwc, nil
		}
	}
	return nil, nil
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
	// Ignore EnvoyProxy not found err.
	gwp.ServiceType, _ = GetGatewayServiceType(context.TODO(), kc, gwc)
	return &gwp, nil
}

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

func GetGatewayServiceType(ctx context.Context, kc client.Client, gwc *gwv1.GatewayClass) (egv1a1.ServiceType, error) {
	key := client.ObjectKey{
		Name:      gwc.Spec.ParametersRef.Name,
		Namespace: string(*gwc.Spec.ParametersRef.Namespace),
	}
	var proxy egv1a1.EnvoyProxy
	if err := kc.Get(ctx, key, &proxy); err != nil {
		klog.Error(err)
		return "", err
	}

	if pv := proxy.Spec.Provider; pv != nil {
		if kub := pv.Kubernetes; kub != nil {
			if svc := kub.EnvoyService; svc != nil {
				if svc.Type != nil {
					return *svc.Type, nil
				}
			}
		}
	}
	return egv1a1.ServiceTypeLoadBalancer, nil
}
