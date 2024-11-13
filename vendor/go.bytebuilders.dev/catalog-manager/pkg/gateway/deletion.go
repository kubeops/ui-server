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

	kmapi "kmodules.xyz/client-go/api/v1"
	cu "kmodules.xyz/client-go/client"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func RemoveListenerOrGateway(c client.Client, gatewayName, gatewayNamespace string, listenerToDelete ...string) error {
	gw := &gwapiv1.Gateway{}
	err := c.Get(context.TODO(), client.ObjectKey{Name: gatewayName, Namespace: gatewayNamespace}, gw)
	if err != nil {
		return err
	}

	keepListeners := make([]gwapiv1.Listener, 0)
	for idx, lisx := range gw.Spec.Listeners {
		if !contains(listenerToDelete, string(lisx.Name)) {
			keepListeners = append(keepListeners, gw.Spec.Listeners[idx])
		}
	}
	if len(keepListeners) == 0 {
		return c.Delete(context.TODO(), gw)
	}
	_, err = cu.CreateOrPatch(context.TODO(), c, gw, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*gwapiv1.Gateway)
		in.Spec.Listeners = keepListeners
		return in
	})
	return err
}

func contains(set []string, str string) bool {
	for _, s := range set {
		if s == str {
			return true
		}
	}
	return false
}

func GetSectionDeletionMap(cr gwapiv1.CommonRouteSpec) map[kmapi.ObjectReference][]string {
	sectionMap := make(map[kmapi.ObjectReference][]string)
	for _, parentRef := range cr.ParentRefs {
		gw := kmapi.ObjectReference{
			Namespace: string(*parentRef.Namespace),
			Name:      string(parentRef.Name),
		}
		set, exists := sectionMap[gw]
		if !exists {
			set = make([]string, 0)
		}
		set = append(set, string(*parentRef.SectionName))
		sectionMap[gw] = set
	}
	return sectionMap
}
