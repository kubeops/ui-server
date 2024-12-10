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
	"strconv"
	"strings"
	"sync"

	catgwapi "go.bytebuilders.dev/catalog/api/gateway/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/net"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type GatewayInfo struct {
	GatewayClassName string
	Ports            []int
}

type PortInfo struct {
	ListenerPort gwv1.PortNumber
	NodePort     gwv1.PortNumber
}

func (pi PortInfo) UsesNodePort() bool {
	return pi.NodePort > 0
}

func (pi PortInfo) String() string {
	return fmt.Sprintf("%d/%d", pi.ListenerPort, pi.NodePort)
}

func ParsePortInfo(str string) (*PortInfo, error) {
	parts := strings.SplitN(str, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid port info: %s", str)
	}
	lp, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid port info: %s", str)
	}
	np, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid port info: %s", str)
	}
	return &PortInfo{
		ListenerPort: gwv1.PortNumber(lp),
		NodePort:     gwv1.PortNumber(np),
	}, nil
}

type ClusterManager struct {
	kc           client.Reader
	portManagers map[string]*GatewayClassPortManager // gwcName -> gatewayClassPortManager
	gwMap        map[string]*GatewayInfo             // ns/name -> {gwclass, []ports}
	svcMgr       *ServicePortManager
	mu           sync.RWMutex
}

func NewClusterManager(kc client.Reader, svcMgr *ServicePortManager) *ClusterManager {
	return &ClusterManager{
		kc:           kc,
		portManagers: map[string]*GatewayClassPortManager{},
		gwMap:        map[string]*GatewayInfo{},
		svcMgr:       svcMgr,
	}
}

func (cm *ClusterManager) UpdateGatewayClass(ctx context.Context, gwp *catgwapi.GatewayParameter) (bool, error) {
	log := log.FromContext(ctx)

	cm.mu.Lock()
	defer cm.mu.Unlock()

	gm, ok := cm.portManagers[gwp.GatewayClassName]
	if !ok {
		var err error
		gm, err = NewGatewayClassPortManager(gwp, cm.svcMgr)
		if err != nil {
			return false, err
		}
		gwMap, err := ListGatewayInfo(context.TODO(), cm.kc, gwp.GatewayClassName)
		if err != nil {
			return false, err
		}
		gm.InitListenerPorts(gwMap)
		for gw, info := range gwMap {
			cm.gwMap[gw] = info
		}

		log.Info("Initialized GatewayClass port manager", "GatewayClass", gwp.GatewayClassName)
		cm.portManagers[gwp.GatewayClassName] = gm
		return true, nil
	}

	updated, err := gm.Update(cm.kc, gwp)
	if err != nil {
		return false, err
	}
	cm.portManagers[gwp.GatewayClassName] = gm
	if updated {
		log.Info("Updated GatewayClass port manager", "GatewayClass", gwp.GatewayClassName)
	}
	return updated, nil
}

func (cm *ClusterManager) DeleteGatewayClass(gatewayClassName string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for gw, info := range cm.gwMap {
		if info.GatewayClassName == gatewayClassName {
			delete(cm.gwMap, gw)
		}
	}
	delete(cm.portManagers, gatewayClassName)
}

func (cm *ClusterManager) UpdateGateway(gw *gwv1.Gateway) (bool, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	return cm.updateGateway(gw)
}

func (cm *ClusterManager) updateGateway(gw *gwv1.Gateway) (bool, error) {
	key := client.ObjectKeyFromObject(gw).String()
	info := ToGatewayInfo(gw)

	gm, found := cm.portManagers[info.GatewayClassName]
	if !found {
		return false, fmt.Errorf("port manager for gateway %q not found", key)
	}

	existing, ok := cm.gwMap[key]
	if !ok {
		gm.MarkAsAllocated(info.Ports)
		cm.gwMap[key] = info
		return true, nil
	}

	if equals(existing.Ports, info.Ports) {
		return false, nil
	}

	gm.UpdatePorts(existing.Ports, info.Ports)
	cm.gwMap[key] = info
	return true, nil
}

func (cm *ClusterManager) DeleteGateway(gw types.NamespacedName) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	key := gw.String()
	existing, ok := cm.gwMap[key]
	if ok {
		gm, found := cm.portManagers[existing.GatewayClassName]
		if !found {
			return fmt.Errorf("port manager for gateway %q not found", key)
		}
		gm.ReleasePorts(existing.Ports)
		delete(cm.gwMap, key)
	}
	return nil
}

func (cm *ClusterManager) GetGatewayClassPortManager(gatewayClassName string) *GatewayClassPortManager {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	return cm.portManagers[gatewayClassName]
}

func (cm *ClusterManager) AllocatePorts(gatewayClassName string, n int) ([]PortInfo, bool, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	gm, ok := cm.portManagers[gatewayClassName]
	if !ok {
		return nil, false, fmt.Errorf("port manager for gateway class %q not found", gatewayClassName)
	}
	return gm.AllocatePorts(n)
}

func (cm *ClusterManager) AllocateSeedPort() (int, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ports, err := cm.svcMgr.AllocatePorts(net.ParsePortRangeOrDie("8080-65535"), 1)
	if err != nil {
		return 0, err
	}
	return ports[0], nil
}

func (cm *ClusterManager) SetSeedPortAllocated(port int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.svcMgr.SetPortAllocated(port)
}

func (cm *ClusterManager) Print() {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, gm := range cm.portManagers {
		gm.Print()
	}

	for gw, info := range cm.gwMap {
		klog.Infof("Gateway=%v, Class=%v, ports=%v \n", gw, info.GatewayClassName, info.Ports)
	}
	cm.svcMgr.Print()
}
