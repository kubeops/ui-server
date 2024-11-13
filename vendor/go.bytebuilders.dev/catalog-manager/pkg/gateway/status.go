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
	"reflect"

	"go.uber.org/multierr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	v1 "sigs.k8s.io/gateway-api/apis/v1"
)

func GetGatewayStatus(c client.Client, gwName, gwNamespace string, listenerNames ...string) error {
	var errs error

	gw := &v1.Gateway{}
	if err := c.Get(context.TODO(), client.ObjectKey{Name: gwName, Namespace: gwNamespace}, gw); err != nil {
		return err
	}

	if !kmapi.IsConditionTrue(gw.Status.Conditions, string(v1.GatewayConditionAccepted)) ||
		!kmapi.IsConditionTrue(gw.Status.Conditions, string(v1.GatewayConditionProgrammed)) {
		errs = multierr.Append(errs, fmt.Errorf("gateway %s/%s not accepted or programmed", gw.Name, gw.Namespace))
	}

	for _, listenerName := range listenerNames {
		conditions := GetListenerCondition(listenerName, gw.Status)
		if !kmapi.IsConditionTrue(conditions, string(gwapiv1.ListenerConditionAccepted)) ||
			!kmapi.IsConditionTrue(conditions, string(gwapiv1.ListenerConditionProgrammed)) ||
			!kmapi.IsConditionTrue(conditions, string(gwapiv1.ListenerConditionResolvedRefs)) {
			errs = multierr.Append(errs, fmt.Errorf("gateway listener \"%s\" not ready, required conditions are not true", listenerName))
		}
	}
	return errs
}

func GetListenerCondition(listenerName string, gwStatus v1.GatewayStatus) []metav1.Condition {
	for _, listener := range gwStatus.Listeners {
		if string(listener.Name) == listenerName {
			return listener.Conditions
		}
	}
	return nil
}

func GetRouteStatus(routes ...any) error {
	var errs error

	// v1alpha1.MySQLRoute{}

	for _, route := range routes {
		rv := reflect.ValueOf(route)
		cr := rv.FieldByName("Status").FieldByName("Parents").Index(0).FieldByName("Conditions")
		conditions := cr.Interface().([]metav1.Condition)
		// status := sr.Interface().(v1.RouteStatus)
		// fmt.Println("the reflected value: ", status)

		// conditions := cr.Interface().()
		if !kmapi.IsConditionTrue(conditions, string(gwapiv1.RouteConditionAccepted)) ||
			!kmapi.IsConditionTrue(conditions, string(gwapiv1.RouteConditionResolvedRefs)) {
			errs = multierr.Append(errs, fmt.Errorf("route \"%s\" not ready, required conditions are not true", rv.FieldByName("Metadata").FieldByName("Name").String()))
		}
	}

	return errs
}
