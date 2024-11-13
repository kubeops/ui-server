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
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServicePortManager struct {
	nodePorts *bits.BitField
	svcMap    map[string][]int // ns/name -> []ports
	mu        sync.RWMutex
}

func NewServicePortManager() *ServicePortManager {
	return &ServicePortManager{
		nodePorts: bits.NewBitField(0xFFFF),
		svcMap:    map[string][]int{},
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

func (sm *ServicePortManager) ReservePorts(pr *net.PortRange, n int) ([]int, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	start := 30000
	end := 32768
	if pr != nil {
		start = pr.Base
		end = pr.Base + pr.Size
	}
	return sm.nodePorts.NextAvailableBitsInRange(start, end, n)
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
	existing, ok := sm.svcMap[key]
	if ok {
		for _, port := range existing {
			sm.nodePorts.ClearBit(port)
		}
		delete(sm.svcMap, key)
	}
}

func (sm *ServicePortManager) update(svc *core.Service) bool {
	key := client.ObjectKeyFromObject(svc).String()
	ports := ListServiceNodePorts(svc)

	existing, ok := sm.svcMap[key]
	if !ok {
		for _, port := range ports {
			sm.nodePorts.SetBit(port)
		}
		sm.svcMap[key] = ports
		return true
	}

	if equals(existing, ports) {
		return false
	}

	for _, port := range existing {
		sm.nodePorts.ClearBit(port)
	}
	for _, port := range ports {
		sm.nodePorts.SetBit(port)
	}
	sm.svcMap[key] = ports
	return true
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
	for svc, ports := range sm.svcMap {
		if len(ports) == 0 {
			continue
		}
		klog.Infof("svc=%v, nodePorts=%v \n", svc, ports)
	}
}
