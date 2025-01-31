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

package gatewayinfo

import (
	"context"
	"fmt"
	"strings"

	"go.bytebuilders.dev/catalog-manager/pkg/gateway"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	"kmodules.xyz/client-go/meta"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Storage struct {
	kc client.Client
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Getter                   = &Storage{}
	_ rest.SingularNameProvider     = &Storage{}
)

func NewStorage(kc client.Client) *Storage {
	return &Storage{
		kc: kc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return rsapi.SchemeGroupVersion.WithKind(rsapi.ResourceKindGatewayInfo)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) GetSingularName() string {
	return strings.ToLower(rsapi.ResourceKindGatewayInfo)
}

// Getter
func (r *Storage) New() runtime.Object {
	return &rsapi.GatewayInfo{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	_, err := r.kc.RESTMapper().RESTMapping(schema.GroupKind{
		Group: gwv1.GroupName,
		Kind:  "GatewayClass",
	})
	if err != nil {
		return nil, fmt.Errorf("kind GatewayClass from %v group is not present in this cluster", gwv1.GroupName)
	}

	class, err := gateway.FindGatewayClass(context.TODO(), r.kc, name)
	if err != nil {
		return nil, err
	}

	const OwningGatewayClassLabel = "gateway.envoyproxy.io/owning-gatewayclass"
	listOpt := &client.ListOptions{}
	if class.Spec.ParametersRef != nil && class.Spec.ParametersRef.Namespace != nil {
		listOpt.Namespace = string(*class.Spec.ParametersRef.Namespace)
	}
	var svcList core.ServiceList
	err = r.kc.List(context.TODO(), &svcList, listOpt)
	if err != nil {
		return nil, err
	}

	var svcType, ip, hostName string
	for _, s := range svcList.Items {
		if s.Labels[meta.ManagedByLabelKey] == "envoy-gateway" && s.Labels[OwningGatewayClassLabel] == class.Name {
			svcType = string(s.Spec.Type)

			val, ok := s.Annotations["external-dns.alpha.kubernetes.io/hostname"]
			if ok {
				hostName = val
			} else {
				if s.Status.LoadBalancer.Ingress != nil {
					ip = s.Status.LoadBalancer.Ingress[0].IP
					hostName = s.Status.LoadBalancer.Ingress[0].Hostname
				}
			}

		}
	}

	return &rsapi.GatewayInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: rsapi.GatewayInfoSpec{
			GatewayClassName: class.Name,
			ServiceType:      svcType,
			HostName:         hostName,
			IP:               ip,
		},
	}, err
}
