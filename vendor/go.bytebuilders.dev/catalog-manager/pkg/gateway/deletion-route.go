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
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	vgapi "voyagermesh.dev/gateway-api/apis/gateway/v1alpha1"
)

type DeletionInfo struct {
	RouteGVK     schema.GroupVersionKind
	DBNamespace  string
	IsTLSEnabled bool
	Services     []string
	RouteNames   []string
}

func CleanupResources(c client.Client, inf DeletionInfo) error {
	if inf.RouteGVK.Group == "" {
		inf.RouteGVK.Group = vgapi.GroupVersion.Group
	}
	if inf.RouteGVK.Version == "" {
		inf.RouteGVK.Version = vgapi.GroupVersion.Version
	}
	for i, service := range inf.Services {
		var route unstructured.Unstructured
		route.SetGroupVersionKind(inf.RouteGVK)
		rName := GetRouteName(service)
		if inf.RouteNames != nil && len(inf.RouteNames) >= i {
			rName = inf.RouteNames[i]
		}
		klog.Infof("11111111111 Deleting route %s/%s", inf.DBNamespace, rName)
		err := c.Get(context.TODO(), client.ObjectKey{Name: rName, Namespace: inf.DBNamespace}, &route)
		if err != nil {
			if apierrors.IsNotFound(err) { // Don't step further if route not found
				continue
			}
			return err
		}
		klog.Infof("22222222222222")
		name, gwNs, err := getParentRef(route)
		if err != nil {
			return err
		}
		klog.Infof("3333333333333333")
		err = RemoveListenerOrGateway(c, name, gwNs, GetListenerName(route.GetName()))
		if err != nil && !apierrors.IsNotFound(err) { // Continuing deletion even if gateway not found
			return err
		}

		klog.Infof("444444444444")

		if err := c.Delete(context.TODO(), &route); err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		klog.Infof("555555555555")
		if inf.IsTLSEnabled {
			btls := &gwapiv1a3.BackendTLSPolicy{}
			err = c.Get(context.TODO(), client.ObjectKey{Name: GetBackendTLSPolicyName(service), Namespace: inf.DBNamespace}, btls)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
			if err := c.Delete(context.TODO(), btls); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		klog.Infof("666666666666666")
	}

	return nil
}

func getParentRef(route unstructured.Unstructured) (string, string, error) {
	parentRefs, found, err := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
	if err != nil || !found || len(parentRefs) == 0 {
		return "", "", errors.New("failed to get parentRefs from spec")
	}

	pRef, ok := parentRefs[0].(map[string]interface{})
	if !ok {
		return "", "", errors.New("failed to parse parentRefs from route spec")
	}

	namespace, found, err := unstructured.NestedString(pRef, "namespace")
	if err != nil || !found {
		return "", "", errors.New("failed to get namespace from parentRef")
	}

	name, found, err := unstructured.NestedString(pRef, "name")
	if err != nil || !found {
		return "", "", errors.New("failed to get name from parentRef")
	}

	return name, namespace, nil
}
