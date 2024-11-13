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

package portmanager

import (
	"context"
	"fmt"
	"sort"
	"sync"

	catgwapi "go.bytebuilders.dev/catalog/api/gateway/v1alpha1"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"gomodules.xyz/bits"
	"k8s.io/apimachinery/pkg/util/net"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type GatewayClassPortManager struct {
	gatewayClassName string
	portRange        string
	serviceType      egv1a1.ServiceType
	nodePortRange    string
	listenerPorts    *bits.PortRange
	nodePorts        *net.PortRange
	svcMgr           *ServicePortManager
	mu               sync.RWMutex
}

func NewGatewayClassPortManager(gwp *catgwapi.GatewayParameter, svcMgr *ServicePortManager) (*GatewayClassPortManager, error) {
	mgr := &GatewayClassPortManager{
		gatewayClassName: gwp.GatewayClassName,
		portRange:        gwp.Service.PortRange,
		nodePortRange:    gwp.Service.NodeportRange,
		svcMgr:           svcMgr,
	}

	if gwp.Service.PortRange != "" {
		pr, err := net.ParsePortRange(gwp.Service.PortRange)
		if err != nil {
			return nil, err
		}
		mgr.listenerPorts, err = bits.NewPortRange(pr.Base, pr.Size)
		if err != nil {
			return nil, err
		}
	}

	if gwp.Service.NodeportRange != "" {
		var err error
		mgr.nodePorts, err = net.ParsePortRange(gwp.Service.NodeportRange)
		if err != nil {
			return nil, err
		}
	}

	return mgr, nil
}

func (gm *GatewayClassPortManager) InitListenerPorts(gwMap map[string]*GatewayInfo) {
	for _, info := range gwMap {
		for _, port := range info.Ports {
			// Skip error if gw has port outside the range.
			// This can happen if the range has been changed after some gw already provisioned.
			_ = gm.listenerPorts.SetPortAllocated(port)
		}
	}
}

func (gm *GatewayClassPortManager) Update(kc client.Reader, gwp *catgwapi.GatewayParameter) (bool, error) {
	if gm.gatewayClassName != gwp.GatewayClassName {
		return false, fmt.Errorf("GatewayClassName mismatch, found: %s, input: %s", gm.gatewayClassName, gwp.GatewayClassName)
	}

	listenerPorts, listenerUpdated, err := func() (*bits.PortRange, bool, error) {
		if gm.portRange == gwp.Service.PortRange {
			return nil, false, nil
		}

		if gwp.Service.PortRange != "" {
			pr, err := net.ParsePortRange(gwp.Service.PortRange)
			if err != nil {
				return nil, false, err
			}
			listenerPorts, err := bits.NewPortRange(pr.Base, pr.Size)
			if err != nil {
				return nil, false, err
			}
			return listenerPorts, true, nil
		}
		return nil, true, nil
	}()
	if err != nil {
		return false, err
	}

	nodePorts, npUpdated, err := func() (*net.PortRange, bool, error) {
		if gm.nodePortRange == gwp.Service.NodeportRange {
			return nil, false, nil
		}

		if gwp.Service.NodeportRange != "" {
			pr, err := net.ParsePortRange(gwp.Service.NodeportRange)
			if err != nil {
				return nil, false, err
			}
			return pr, true, nil
		}
		return nil, true, nil
	}()
	if err != nil {
		return false, err
	}

	var gwMap map[string]*GatewayInfo
	if listenerPorts != nil {
		gwMap, err = ListGatewayInfo(context.TODO(), kc, gwp.GatewayClassName)
		if err != nil {
			return false, err
		}
	}

	gm.mu.Lock()
	defer gm.mu.Unlock()

	if listenerUpdated {
		gm.listenerPorts = listenerPorts
		gm.portRange = gwp.Service.PortRange
		if listenerPorts != nil {
			gm.InitListenerPorts(gwMap)
		}
	}
	if npUpdated {
		gm.nodePorts = nodePorts
		gm.nodePortRange = gwp.Service.NodeportRange
	}
	return listenerUpdated || npUpdated, nil
}

func (gm *GatewayClassPortManager) GatewayClassName() string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	return gm.gatewayClassName
}

func (gm *GatewayClassPortManager) PortRange() string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	return gm.portRange
}

func (gm *GatewayClassPortManager) ServiceType() egv1a1.ServiceType {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	return gm.serviceType
}

func (gm *GatewayClassPortManager) NodePortRange() string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	return gm.nodePortRange
}

func (gm *GatewayClassPortManager) ReservePorts(n int) ([]PortInfo, bool, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	ports, err := gm.listenerPorts.AllocateNextPorts(n)
	if err != nil {
		return nil, false, err
	}

	var nodeports []int
	usesNodePort := gm.UsesNodePort()
	if usesNodePort {
		nodeports, err = gm.svcMgr.ReservePorts(gm.nodePorts, n)
		if err != nil {
			return nil, false, err
		}
	}

	result := make([]PortInfo, n)
	for i := 0; i < n; i++ {
		result[i] = PortInfo{ListenerPort: gwv1.PortNumber(ports[i])}
		if usesNodePort {
			result[i].NodePort = gwv1.PortNumber(nodeports[i])
		}
	}
	return result, usesNodePort, nil
}

func (gm *GatewayClassPortManager) UsesNodePort() bool {
	return gm.serviceType == egv1a1.ServiceTypeNodePort ||
		gm.serviceType == egv1a1.ServiceTypeLoadBalancer
}

func (gm *GatewayClassPortManager) MarkAsAllocated(ports []int) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.listenerPorts == nil {
		return
	}
	for _, port := range ports {
		// ignore error if port is out of range
		_ = gm.listenerPorts.SetPortAllocated(port)
	}
}

func (gm *GatewayClassPortManager) UpdatePorts(old, cur []int) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.listenerPorts == nil {
		return
	}
	// ignore error if port is out of range
	_ = gm.listenerPorts.ReleasePorts(old)
	for _, port := range cur {
		// ignore error if port is out of range
		_ = gm.listenerPorts.SetPortAllocated(port)
	}
}

func (gm *GatewayClassPortManager) ReleasePorts(ports []int) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.listenerPorts == nil {
		return
	}
	// ignore error if port is out of range
	_ = gm.listenerPorts.ReleasePorts(ports)
}

func ListGatewayInfo(ctx context.Context, kc client.Reader, gatewayClassName string) (map[string]*GatewayInfo, error) {
	var list gwv1.GatewayList
	err := kc.List(ctx, &list)
	if err != nil {
		return nil, err
	}

	gwMap := map[string]*GatewayInfo{}
	for _, gw := range list.Items {
		if string(gw.Spec.GatewayClassName) == gatewayClassName {
			gwMap[client.ObjectKeyFromObject(&gw).String()] = ToGatewayInfo(&gw)
		}
	}
	return gwMap, nil
}

func ToGatewayInfo(gw *gwv1.Gateway) *GatewayInfo {
	ports := make([]int, 0, len(gw.Spec.Listeners))
	for _, l := range gw.Spec.Listeners {
		ports = append(ports, int(l.Port))
	}
	sort.Ints(ports)

	return &GatewayInfo{
		GatewayClassName: string(gw.Spec.GatewayClassName),
		Ports:            ports,
	}
}

func (gm *GatewayClassPortManager) Print() {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	klog.Infof("gatewayClass=%v, portRange=%v, nodeportRange=%v, svcType=%v\n",
		gm.gatewayClassName, gm.portRange, gm.nodePortRange, gm.serviceType)
}
