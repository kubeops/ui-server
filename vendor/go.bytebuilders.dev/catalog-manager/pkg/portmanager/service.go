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
	"sort"
	"sync"

	"gomodules.xyz/bits"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PortAlloc int

const (
	PortAllocUnknown PortAlloc = iota - 1
	PortAllocUnique
	PortAllocShared
)

type ServicePortManager struct {
	kc client.Reader

	ports   *bits.BitField
	portMap map[string][]int // ns/name -> []ports

	nodePorts *bits.BitField
	npMap     map[string][]int // ns/name -> []ports
	mu        sync.RWMutex

	algPA PortAlloc
	muPA  sync.Mutex
}

func NewServicePortManager(kc client.Reader) *ServicePortManager {
	return &ServicePortManager{
		kc:        kc,
		ports:     bits.NewBitField(0xFFFF + 1),
		portMap:   map[string][]int{},
		nodePorts: bits.NewBitField(0xFFFF + 1),
		npMap:     map[string][]int{},
		algPA:     PortAllocUnknown,
	}
}

func (sm *ServicePortManager) Init(kc client.Reader) error {
	var list core.ServiceList
	err := kc.List(context.TODO(), &list)
	if err != nil {
		return err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, svc := range list.Items {
		sm.update(&svc)
	}
	return nil
}

func (sm *ServicePortManager) AllocatePorts(pr *net.PortRange, n int) ([]int, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.ports.AllocateAvailableBitsInRange(pr.Base, pr.Base+pr.Size, n)
}

func (sm *ServicePortManager) SetPortAllocated(port int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.ports.SetBit(port)
}

func (sm *ServicePortManager) AllocateNodePorts(pr *net.PortRange, n int) ([]int, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.nodePorts.AllocateAvailableBitsInRange(pr.Base, pr.Base+pr.Size, n)
}

func (sm *ServicePortManager) Update(svc *core.Service) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.update(svc)
}

func (sm *ServicePortManager) Delete(svc types.NamespacedName) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	key := svc.String()

	if existing, ok := sm.portMap[key]; ok {
		for _, port := range existing {
			sm.ports.ClearBit(port)
		}
		delete(sm.portMap, key)
	}

	if existing, ok := sm.npMap[key]; ok {
		for _, port := range existing {
			sm.nodePorts.ClearBit(port)
		}
		delete(sm.npMap, key)
	}
}

func (sm *ServicePortManager) update(svc *core.Service) bool {
	key := client.ObjectKeyFromObject(svc).String()

	return updatePorts(sm.ports, sm.portMap, key, ListServicePorts(svc)) ||
		updatePorts(sm.nodePorts, sm.npMap, key, ListServiceNodePorts(svc))
}

func updatePorts(ports *bits.BitField, portMap map[string][]int, svcKey string, svcPorts []int) bool {
	existing, ok := portMap[svcKey]
	if !ok {
		for _, port := range svcPorts {
			ports.SetBit(port)
		}
		portMap[svcKey] = svcPorts
		return true
	}

	if equals(existing, svcPorts) {
		return false
	}

	for _, port := range existing {
		ports.ClearBit(port)
	}
	for _, port := range svcPorts {
		ports.SetBit(port)
	}
	portMap[svcKey] = svcPorts
	return true
}

func ListServicePorts(svc *core.Service) []int {
	if svc.Spec.Type == core.ServiceTypeLoadBalancer || len(svc.Spec.ExternalIPs) > 0 {
		ports := make([]int, 0, len(svc.Spec.Ports))
		for _, port := range svc.Spec.Ports {
			ports = append(ports, int(port.Port))
		}
		sort.Ints(ports)
		return ports
	}

	return nil
}

func ListServiceNodePorts(svc *core.Service) []int {
	ports := make([]int, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		if port.NodePort != 0 {
			ports = append(ports, int(port.NodePort))
		}
	}
	sort.Ints(ports)
	return ports
}

func equals(x, y []int) bool {
	if len(x) != len(y) {
		return false
	}
	for i := 0; i < len(x); i++ {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func (sm *ServicePortManager) Print() {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for svc, ports := range sm.npMap {
		if len(ports) == 0 {
			continue
		}
		klog.Infof("svc=%v, nodePorts=%v \n", svc, ports)
	}
}

func (sm *ServicePortManager) GetPortAlloc() (PortAlloc, error) {
	if sm.algPA != PortAllocUnknown {
		return sm.algPA, nil
	}
	sm.muPA.Lock()
	defer sm.muPA.Unlock()

	var gwsvcList core.ServiceList
	/*
		app.kubernetes.io/component: proxy
		app.kubernetes.io/managed-by: envoy-gateway
		app.kubernetes.io/name: envoy
		gateway.envoyproxy.io/owning-gatewayclass: ace
	*/
	err := sm.kc.List(context.TODO(), &gwsvcList, client.MatchingLabels{
		"app.kubernetes.io/component":  "proxy",
		"app.kubernetes.io/managed-by": "envoy-gateway",
		"app.kubernetes.io/name":       "envoy",
	})
	if err != nil {
		return sm.algPA, err
	}
	for _, svc := range gwsvcList.Items {
		if len(svc.Spec.ExternalIPs) > 0 {
			sm.algPA = PortAllocShared
			return sm.algPA, nil
		}
	}

	var list core.ServiceList
	err = sm.kc.List(context.TODO(), &list)
	if err != nil {
		return sm.algPA, err
	}

	baseline := sets.New[string]()
	for _, svc := range gwsvcList.Items {
		if svc.Spec.Type != core.ServiceTypeLoadBalancer {
			continue
		}

		hosts := sets.New[string]()
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.IP != "" {
				hosts.Insert(ing.IP)
			}
			if ing.Hostname != "" {
				hosts.Insert(ing.Hostname)
			}
		}
		if hosts.Len() > 0 {
			if baseline.Len() == 0 {
				baseline = hosts
			} else {
				if baseline.Intersection(hosts).Len() > 0 {
					sm.algPA = PortAllocShared
				} else {
					sm.algPA = PortAllocUnique
				}
				return sm.algPA, nil
			}
		}
	}
	return sm.algPA, nil
}
