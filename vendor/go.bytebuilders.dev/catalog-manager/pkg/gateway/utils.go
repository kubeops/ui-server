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
	"math/rand"
	"strconv"
	"strings"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	urand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog/v2"
	"k8s.io/utils/net"
	cu "kmodules.xyz/client-go/client"
	ofst "kmodules.xyz/offshoot-api/api/v1"
	kubedbv1 "kubedb.dev/apimachinery/apis/kubedb/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	vgapi "voyagermesh.dev/gateway-api/apis/gateway/v1alpha1"
)

const (
	GatewayNameAnnotation      = "gateway.networking.k8s.io/name"
	GatewayNamespaceAnnotation = "gateway.networking.k8s.io/namespace"

	NamespaceLabel   = "kubernetes.io/metadata.name"
	GatewayResources = "GatewayResources"
	GatewayIP        = "GatewayIP"
	GatewayPort      = "GatewayPort"

	Available = "Available"

	ResourceKindGateway = "Gateway"

	ResourceKindMongoDBRoute = "MongoDBRoute"
	ResourceKindMySQLRoute   = "MySQLRoute"
	ResourceKindRedisRoute   = "RedisRoute"
	ResourceKindHTTPRoute    = "HTTPRoute"

	ResourceKindMongoDBBinding       = "MongoDBBinding"
	ResourceKindMySQLBinding         = "MySQLBinding"
	ResourceKindRedisBinding         = "RedisBinding"
	ResourceKindElasticsearchBinding = "ElasticsearchBinding"

	ApiGroupK8sGateway     = "gateway.networking.k8s.io"
	ApiGroupVoyagerGateway = "gateway.voyagermesh.com"

	PortMappingKeyPrefix = "port-mapping.gateway.voyagermesh.com/"
)

func GatewayAvailable(c client.Client, bindAnnot map[string]string) bool {
	if bindAnnot == nil {
		return false
	}
	if bindAnnot[GatewayNameAnnotation] == "" || bindAnnot[GatewayNamespaceAnnotation] == "" {
		klog.Info("no proper gateway mentioned")
	}
	gwName := bindAnnot[GatewayNameAnnotation]
	gwNamespace := bindAnnot[GatewayNamespaceAnnotation]
	gw := &gwv1.Gateway{}
	err := c.Get(context.TODO(), client.ObjectKey{Name: gwName, Namespace: gwNamespace}, gw)
	if apierrors.IsNotFound(err) {
		klog.Info("no gateway found")
		return false
	} else if err != nil {
		klog.Error(err)
		return false
	}
	return true
}

func RemoveLabelFromMongoDBRoute(c client.Client, route *vgapi.MongoDBRoute, keystring string) error {
	_, err := cu.CreateOrPatch(context.TODO(), c, route, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*vgapi.MongoDBRoute)
		delete(in.Labels, keystring)
		return in
	})
	return err
}

func RemoveLabelFromMySQLRoute(c client.Client, route *vgapi.MySQLRoute, keystring string) error {
	_, err := cu.CreateOrPatch(context.TODO(), c, route, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*vgapi.MySQLRoute)
		delete(in.Labels, keystring)
		return in
	})
	return err
}

func RemoveLabelFromRedisRoute(c client.Client, route *vgapi.RedisRoute, keystring string) error {
	_, err := cu.CreateOrPatch(context.TODO(), c, route, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*vgapi.RedisRoute)
		delete(in.Labels, keystring)
		return in
	})
	return err
}

func RemoveLabelFromHTTPRoute(c client.Client, route *gwv1.HTTPRoute, keystring string) error {
	_, err := cu.CreateOrPatch(context.TODO(), c, route, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*gwv1.HTTPRoute)
		delete(in.Labels, keystring)
		return in
	})
	return err
}

func RemoveLabelFromReferenceGrant(c client.Client, refg *gwv1b1.ReferenceGrant, keystring string) error {
	_, err := cu.CreateOrPatch(context.TODO(), c, refg, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*gwv1b1.ReferenceGrant)
		delete(in.Labels, keystring)
		return in
	})
	return err
}

func RemoveListener(c client.Client, reference gwv1.ParentReference) error {
	gatewayObjKey := client.ObjectKey{
		Namespace: string(*reference.Namespace),
		Name:      string(reference.Name),
	}

	gatewayObj := &gwv1.Gateway{}

	err := c.Get(context.TODO(), gatewayObjKey, gatewayObj)
	if err != nil {
		return err
	}

	_, err = cu.CreateOrPatch(context.TODO(), c, gatewayObj, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*gwv1.Gateway)
		for idx, lis := range in.Spec.Listeners {
			if string(lis.Name) == string(*reference.SectionName) {
				in.Spec.Listeners = append(in.Spec.Listeners[:idx], in.Spec.Listeners[idx+1:]...)
				break
			}
		}
		return in
	})

	return err
}

func GetExistingPorts(listeners []gwv1.Listener) []gwv1.PortNumber {
	portList := []gwv1.PortNumber{}
	for _, listener := range listeners {
		portList = append(portList, listener.Port)
	}
	return portList
}

func PatchGateway(c client.Client, routeKind, routeNamespace string, parentRef gwv1.ParentReference) error {
	gateways := gwv1.GatewayList{}
	err := c.List(context.TODO(), &gateways)
	if err != nil {
		klog.Error(err)
		return nil
	}

	for _, gw := range gateways.Items {
		if gw.Name == string(parentRef.Name) {
			if gw.Namespace == string(*parentRef.Namespace) {
				listeners := gw.Spec.Listeners
				for _, listener := range listeners {
					if listener.Name == *parentRef.SectionName {
						return nil
					}
				}
				_, err := cu.CreateOrPatch(context.TODO(), c, &gw, func(obj client.Object, createOp bool) client.Object {
					in := obj.(*gwv1.Gateway)
					in.Spec.Listeners = append(in.Spec.Listeners, *GetListener(parentRef.SectionName, routeKind, routeNamespace, GetExistingPorts(listeners)))
					return in
				})
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func GetListener(sectionName *gwv1.SectionName, routeKind, routeNamespace string, existingPorts []gwv1.PortNumber) *gwv1.Listener {
	lis := &gwv1.Listener{
		Name: *sectionName,
		Port: GetNewPort(existingPorts),
		Protocol: func() gwv1.ProtocolType {
			switch routeKind {
			case ResourceKindHTTPRoute:
				return gwv1.HTTPProtocolType
			default:
				return gwv1.TCPProtocolType
			}
		}(),
		AllowedRoutes: &gwv1.AllowedRoutes{
			Namespaces: &gwv1.RouteNamespaces{
				From: func() *gwv1.FromNamespaces {
					var x gwv1.FromNamespaces = gwv1.NamespacesFromSelector
					return &x
				}(),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						NamespaceLabel: routeNamespace,
					},
				},
			},
			Kinds: nil,
		},
	}
	return lis
}

func GetNewPort(existingPorts []gwv1.PortNumber) gwv1.PortNumber {
	for {
		newPort := gwv1.PortNumber(1024 + rand.Int31n(40000))
		ok := true
		for _, port := range existingPorts {
			if newPort == port {
				ok = false
			}
		}
		if !ok {
			continue
		}
		return newPort
	}
}

func ExistingRefg(c client.Client, bindKind, gwNamespace, routeKind, routeName, routeNamespace string) *gwv1b1.ReferenceGrant {
	refgs := gwv1b1.ReferenceGrantList{}
	err := c.List(context.Background(), &refgs)
	if err != nil {
		klog.Error(err)
		return nil
	}
	for _, refg := range refgs.Items {
		if refg.Namespace == routeNamespace {
			if string(refg.Spec.From[0].Kind) == ResourceKindGateway {
				if string(refg.Spec.From[0].Namespace) == gwNamespace {
					if string(refg.Spec.To[0].Kind) == routeKind {
						if string(*refg.Spec.To[0].Name) == routeName && refg.Labels != nil {
							for key := range refg.Labels {
								if strings.Contains(key, bindKind) {
									return &refg
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func CreateReferenceGrant(c client.Client, bind client.Object, route client.Object) (*gwv1b1.ReferenceGrant, error) {
	refg := &gwv1b1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      urand.String(10),
			Namespace: route.GetNamespace(),
			Labels: map[string]string{
				GetLabelKey(bind): Available,
			},
		},
		Spec: gwv1b1.ReferenceGrantSpec{
			From: []gwv1b1.ReferenceGrantFrom{
				{
					Group: ApiGroupK8sGateway,
					Kind:  ResourceKindGateway,
					Namespace: func() gwv1.Namespace {
						annot := bind.GetAnnotations()
						return gwv1.Namespace(annot[GatewayNamespaceAnnotation])
					}(),
				},
			},
			To: []gwv1b1.ReferenceGrantTo{
				{
					Group: func() gwv1.Group {
						switch bind.GetObjectKind().GroupVersionKind().Kind {
						case ResourceKindElasticsearchBinding:
							return ApiGroupK8sGateway
						default:
							return ApiGroupVoyagerGateway
						}
					}(),
					Kind: func() gwv1.Kind {
						switch bind.GetObjectKind().GroupVersionKind().Kind {
						case ResourceKindMongoDBBinding:
							return ResourceKindMongoDBRoute
						case ResourceKindMySQLBinding:
							return ResourceKindMySQLRoute
						case ResourceKindRedisBinding:
							return ResourceKindRedisRoute
						default:
							return ResourceKindHTTPRoute
						}
					}(),
					Name: func() *gwv1.ObjectName {
						x := gwv1.ObjectName(route.GetName())
						return &x
					}(),
				},
			},
		},
	}

	_, err := cu.CreateOrPatch(context.TODO(), c, refg, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*gwv1b1.ReferenceGrant)
		return in
	})
	if err != nil {
		return nil, err
	}

	return refg, err
}

func GetCommonLabelKey(bind client.Object) string {
	return bind.GetObjectKind().GroupVersionKind().Kind
}

func GetLabelKey(bind client.Object) string {
	bindName := bind.GetName()
	bindNamespace := bind.GetNamespace()
	return fmt.Sprintf("%s.%s/%s", bindName, bindNamespace, GetCommonLabelKey(bind))
	// return label
}

func PatchLabelToRefg(c client.Client, refg *gwv1b1.ReferenceGrant, labelKey string) error {
	_, err := cu.CreateOrPatch(context.TODO(), c, refg, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*gwv1b1.ReferenceGrant)
		if in.Labels == nil {
			in.Labels = map[string]string{
				labelKey: Available,
			}
		} else {
			in.Labels[labelKey] = Available
		}
		return in
	})
	if err != nil {
		klog.Error(err)
		return err
	}
	return nil
}

func GetIpPort(c client.Client, reference gwv1.ParentReference) (string, string, error) {
	gw := &gwv1.Gateway{}
	err := c.Get(context.TODO(), client.ObjectKey{Namespace: string(*reference.Namespace), Name: string(reference.Name)}, gw)
	if err != nil {
		return "", "", err
	}

	ip := gw.Status.Addresses[0].Value
	port := ""

	for _, lis := range gw.Spec.Listeners {
		if lis.Name == *reference.SectionName {
			port = strconv.Itoa(int(lis.Port))
		}
	}

	if port == "" {
		err = fmt.Errorf("no listener found on the gateway")
	}

	return ip, port, err
}

func GroupPtr(name string) *gwv1.Group {
	group := gwv1.Group(name)
	return &group
}

func KindPtr(name string) *gwv1.Kind {
	kind := gwv1.Kind(name)
	return &kind
}

func NamespacePtr(name string) *gwv1.Namespace {
	namespace := gwv1.Namespace(name)
	return &namespace
}

func SectionNamePtr(name string) *gwv1.SectionName {
	sectionName := gwv1.SectionName(name)
	return &sectionName
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

//func GetNamedServiceStatus(serviceAlias kubedbv1.ServiceAlias, serviceName string, databasePort int32, gw *gwapiv1.Gateway) api.NamedServiceStatus {
//	nss := api.NamedServiceStatus{
//		Alias: string(serviceAlias),
//		Ports: []ofst.GatewayPort{
//			{
//				BackendServicePort: databasePort,
//				Port: func() int32 {
//					for _, lis := range gw.Spec.Listeners {
//						if lis.Name == gwapiv1b1.SectionName(GetListenerName(GetRouteName(serviceName))) {
//							return int32(lis.Port)
//						}
//					}
//					return 0
//				}(),
//				NodePort: func() int32 {
//					if !portmanager.NodePortEnabled {
//						return 0
//					}
//					for _, lis := range gw.Spec.Listeners {
//						if lis.Name == gwapiv1b1.SectionName(GetListenerName(GetRouteName(serviceName))) {
//							port := int32(lis.Port)
//							nodePort, _ := strconv.Atoi(gw.Annotations[fmt.Sprint("port-mapping.gateway.voyagermesh.com", "/", port)])
//							return int32(nodePort)
//						}
//					}
//					return 0
//				}(),
//			},
//		},
//	}
//	return nss
//}

///new code

type MatchType string

const (
	MatchTypeExact  MatchType = "Exact"
	MatchTypePrefix MatchType = "Prefix"
)

func GetNamedServiceStatus(serviceAlias kubedbv1.ServiceAlias, serviceName string, databasePort int32, gw *gwv1.Gateway, matchType ...MatchType) ofst.NamedServiceStatus {
	var ports []ofst.GatewayPort
	lisNamePrefix := GetListenerName(GetRouteName(serviceName))

	for _, lis := range gw.Spec.Listeners {
		matched := false
		if matchType != nil && matchType[0] == MatchTypePrefix {
			if strings.HasPrefix(string(lis.Name), lisNamePrefix) {
				matched = true
			}
		}
		if matched || string(lis.Name) == lisNamePrefix {
			port := ofst.GatewayPort{
				BackendServicePort: databasePort,
				Port:               int32(lis.Port),
				NodePort: func() int32 {
					if np, found := gw.Annotations[fmt.Sprintf("%s%d", PortMappingKeyPrefix, lis.Port)]; found {
						nodePort, _ := strconv.Atoi(np)
						return int32(nodePort)
					}
					return 0
				}(),
			}
			ports = append(ports, port)
		}
	}

	nss := ofst.NamedServiceStatus{
		Alias: string(serviceAlias),
		Ports: ports,
	}
	return nss
}

func GetBindGatewayStatus(gwName, gwNamespace, gwIP, gwAddress string, namedServices []ofst.NamedServiceStatus, uiUrl []ofst.NamedURL) *ofst.Gateway {
	if namedServices == nil {
		return nil
	}

	gatewayStatus := &ofst.Gateway{
		Name:      gwName,
		Namespace: gwNamespace,
		IP:        gwIP,
		Hostname: func() string {
			if net.IsIPv4String(gwAddress) {
				return ""
			}
			return gwAddress
		}(),
		Services: namedServices,
	}
	for _, ui := range uiUrl {
		temp := ofst.NamedURL{
			Alias:       ui.Alias,
			URL:         ui.URL,
			Port:        ui.Port,
			HelmRelease: ui.HelmRelease,
		}
		gatewayStatus.UI = append(gatewayStatus.UI, temp)
	}
	return gatewayStatus
}

func GetGatewayIP(gateway *gwv1.Gateway) string {
	for _, tp := range gateway.Status.Addresses {
		if tp.Type != nil && *tp.Type == gwv1.IPAddressType {
			return tp.Value
		}
	}
	return ""
}

func GetGatewayAddress(c client.Client, ctx context.Context, gw *gwv1.Gateway) (string, error) {
	ip := GetGatewayIP(gw)
	if ip == "" {
		return "", fmt.Errorf("no ip assigned yet")
	}

	gc := &gwv1.GatewayClass{}
	gcName := string(gw.Spec.GatewayClassName)
	if err := c.Get(ctx, client.ObjectKey{Name: gcName}, gc); err != nil {
		return "", err
	}

	eproxyName := gc.Spec.ParametersRef.Name
	eproxyNamespace := string(*gc.Spec.ParametersRef.Namespace)

	eproxy := &egv1a1.EnvoyProxy{}
	if err := c.Get(ctx, client.ObjectKey{Name: eproxyName, Namespace: eproxyNamespace}, eproxy); err != nil {
		klog.Error(err)
		return "", err
	}

	if eproxy.Spec.Provider != nil {
		if eproxy.Spec.Provider.Kubernetes != nil {
			if eproxy.Spec.Provider.Kubernetes.EnvoyService != nil {
				if eproxy.Spec.Provider.Kubernetes.EnvoyService.Annotations != nil {
					if val, ok := eproxy.Spec.Provider.Kubernetes.EnvoyService.Annotations["external-dns.alpha.kubernetes.io/hostname"]; ok {
						return val, nil
					}
				}
			}
		}
	}
	return ip, nil
}
