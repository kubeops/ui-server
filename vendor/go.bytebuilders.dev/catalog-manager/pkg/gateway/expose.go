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

	"go.bytebuilders.dev/catalog-manager/pkg/portmanager"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cu "kmodules.xyz/client-go/client"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func EnsureReferenceGrants(c client.Client, refgName, refgNamespace string, to gwapiv1b1.ReferenceGrantTo, from gwapiv1b1.ReferenceGrantFrom) error {
	refg := &gwapiv1b1.ReferenceGrant{}
	if err := c.Get(context.TODO(), client.ObjectKey{Name: refgName, Namespace: refgNamespace}, refg); err != nil {
		if apierrors.IsNotFound(err) {
			refg = &gwapiv1b1.ReferenceGrant{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      refgName,
					Namespace: refgNamespace,
				},
				Spec: gwapiv1b1.ReferenceGrantSpec{},
			}
		} else {
			return err
		}
	}

	if refg.Spec.To == nil {
		refg.Spec.To = []gwapiv1b1.ReferenceGrantTo{to}
	} else {
		for _, cur := range refg.Spec.To {
			if cur.Group == to.Group && cur.Kind == to.Kind && *cur.Name == *to.Name {
				goto addFrom
			}
		}
		refg.Spec.To = append(refg.Spec.To, to)
	}

addFrom:
	if refg.Spec.From == nil {
		refg.Spec.From = []gwapiv1b1.ReferenceGrantFrom{from}
	} else {
		for _, cur := range refg.Spec.From {
			if cur == from {
				goto createRefg
			}
		}
		refg.Spec.From = append(refg.Spec.From, from)
	}

createRefg:
	_, err := cu.CreateOrPatch(context.TODO(), c, refg, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*gwapiv1b1.ReferenceGrant)
		in.Spec = refg.Spec
		return in
	})
	return err
}

func GetGateway(c client.Client, gwName, gwNamespace, gwClassName string) (*gwapiv1.Gateway, error) {
	gw := &gwapiv1.Gateway{}
	if err := c.Get(context.TODO(), client.ObjectKey{Name: gwName, Namespace: gwNamespace}, gw); err != nil {
		if apierrors.IsNotFound(err) {
			gw = &gwapiv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwName,
					Namespace: gwNamespace,
				},
				Spec: gwapiv1.GatewaySpec{
					GatewayClassName: gwapiv1.ObjectName(gwClassName),
				},
			}
		} else {
			return nil, err
		}
	}
	gw.Spec.GatewayClassName = gwapiv1.ObjectName(gwClassName)
	return gw, nil
}

func PatchListener(cm *portmanager.ClusterManager, gw *gwapiv1.Gateway, lisName string, routeKind string, https bool, certRefs ...gwapiv1a2.SecretObjectReference) error {
	if gw.Spec.Listeners == nil {
		gw.Spec.Listeners = []gwapiv1b1.Listener{}
	}

	var (
		lisIndex                        = -1
		listenerPort gwapiv1.PortNumber = -1
	)
	alreadyListenerExists := func() bool { return lisIndex != -1 }

	for i, lis := range gw.Spec.Listeners {
		if string(lis.Name) == lisName {
			lisIndex = i
			listenerPort = lis.Port
		}
	}

	if !alreadyListenerExists() {
		ports, usesNodePort, err := cm.AllocatePorts(string(gw.Spec.GatewayClassName), 1)
		if err != nil {
			return err
		}
		if len(ports) == 0 {
			return fmt.Errorf("can't allocate ports for gatewayClass %s", gw.Spec.GatewayClassName)
		}
		if usesNodePort {
			patchPortMappingAnnotation(gw, ports[0])
		}
		listenerPort = ports[0].ListenerPort
	}

	listener := constructListener(lisName, routeKind, listenerPort, https, certRefs...)
	if alreadyListenerExists() {
		gw.Spec.Listeners[lisIndex] = *listener
	} else {
		gw.Spec.Listeners = append(gw.Spec.Listeners, *listener)
	}
	return nil
}

func patchPortMappingAnnotation(gw *gwapiv1.Gateway, portInfo portmanager.PortInfo) {
	portMapping := fmt.Sprintf("%s%d", PortMappingKeyPrefix, portInfo.ListenerPort)
	nodePort := fmt.Sprintf("%d", portInfo.NodePort)
	if gw.Annotations == nil {
		gw.SetAnnotations(map[string]string{
			portMapping: nodePort,
		})
	} else {
		gw.Annotations[portMapping] = nodePort
	}
}

func constructListener(listenerName, routeKind string, port gwapiv1.PortNumber, https bool, certRef ...gwapiv1a2.SecretObjectReference) *gwapiv1b1.Listener {
	lis := &gwapiv1b1.Listener{
		Name: gwapiv1.SectionName(listenerName),
		Port: port,
		Protocol: func() gwapiv1b1.ProtocolType {
			if https {
				return gwapiv1.HTTPSProtocolType
			} else if len(certRef) == 0 || string(certRef[0].Name) == "" {
				return gwapiv1.TCPProtocolType
			}
			return gwapiv1.TLSProtocolType
		}(),
		AllowedRoutes: &gwapiv1b1.AllowedRoutes{
			Namespaces: &gwapiv1b1.RouteNamespaces{
				From: func() *gwapiv1b1.FromNamespaces {
					x := gwapiv1.NamespacesFromSame
					return &x
				}(),
			},
			Kinds: []gwapiv1b1.RouteGroupKind{
				{
					Group: func() *gwapiv1.Group {
						switch routeKind {
						case "HTTPRoute":
							return GroupPtr("gateway.networking.k8s.io")
						case "TCPRoute":
							return GroupPtr("gateway.networking.k8s.io")
						default:
							return GroupPtr("gateway.voyagermesh.com")
						}
					}(),
					Kind: gwapiv1b1.Kind(routeKind),
				},
			},
		},
		TLS: func() *gwapiv1.ListenerTLSConfig {
			if len(certRef) == 0 || string(certRef[0].Name) == "" {
				return nil
			}
			tlsConfig := &gwapiv1.ListenerTLSConfig{
				Mode: func() *gwapiv1.TLSModeType {
					x := gwapiv1.TLSModeTerminate
					return &x
				}(),
				CertificateRefs: certRef,
			}
			return tlsConfig
		}(),
	}
	return lis
}

func EnsureBackendTLSPolicy(c client.Client, serviceName, tlsSecretName, namespace string) error {
	btp := &gwapiv1.BackendTLSPolicy{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetBackendTLSPolicyName(serviceName), // todo fix naming
			Namespace: namespace,
		},
		Spec: gwapiv1.BackendTLSPolicySpec{
			TargetRefs: []gwapiv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gwapiv1.LocalPolicyTargetReference{
						Group: "",
						Kind:  "Service",
						Name:  gwapiv1a2.ObjectName(serviceName),
					},
					SectionName: nil,
				},
			},
			Validation: gwapiv1.BackendTLSPolicyValidation{
				CACertificateRefs: []gwapiv1a2.LocalObjectReference{
					{
						Group: "",
						Kind:  "Secret",
						Name:  gwapiv1a2.ObjectName(tlsSecretName),
					},
				},
				Hostname: "unused",
			},
		},
	}

	_, err := cu.CreateOrPatch(context.TODO(), c, btp, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*gwapiv1.BackendTLSPolicy)
		return in
	})
	return err
}
