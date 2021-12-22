/*
Copyright AppsCode Inc. and Contributors.

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

package graph

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiv1 "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	setx "kmodules.xyz/resource-metadata/pkg/utils/sets"
)

type ObjectGraph struct {
	m     sync.RWMutex
	edges map[apiv1.OID]map[v1alpha1.EdgeLabel]setx.OID // oid -> label -> edges
	ids   map[apiv1.OID]map[v1alpha1.EdgeLabel]setx.OID // oid -> label -> edges
}

func (g *ObjectGraph) Update(src apiv1.OID, connsPerLabel map[v1alpha1.EdgeLabel]setx.OID) {
	g.m.Lock()
	defer g.m.Unlock()

	for lbl, conns := range connsPerLabel {

		if oldConnsPerLabel, ok := g.ids[src]; ok {
			if oldConns, ok := oldConnsPerLabel[lbl]; ok {
				if oldConns.Difference(conns).Len() == 0 {
					return
				}

				g.edges[src][lbl].Delete(oldConns.UnsortedList()...)
				for dst := range oldConns {
					g.edges[dst][lbl].Delete(src)
				}
			}
		}

		if _, ok := g.edges[src]; !ok {
			g.edges[src] = map[v1alpha1.EdgeLabel]setx.OID{}
		}
		if _, ok := g.edges[src][lbl]; !ok {
			g.edges[src][lbl] = setx.NewOID()
		}
		g.edges[src][lbl].Insert(conns.UnsortedList()...)

		for dst := range conns {
			if _, ok := g.edges[dst]; !ok {
				g.edges[dst] = map[v1alpha1.EdgeLabel]setx.OID{}
			}
			if _, ok := g.edges[dst][lbl]; !ok {
				g.edges[dst][lbl] = setx.NewOID()
			}
			g.edges[dst][lbl].Insert(src)
		}
	}

	g.ids[src] = connsPerLabel
}

func (g *ObjectGraph) Links(oid *apiv1.ObjectID, edgeLabel v1alpha1.EdgeLabel) (map[metav1.GroupKind][]apiv1.ObjectID, error) {
	g.m.RLock()
	defer g.m.RUnlock()

	if edgeLabel == v1alpha1.EdgeOffshoot {
		return g.links(oid, nil, edgeLabel)
	}

	src := oid.OID()
	offshoots := g.connectedOIDs([]apiv1.OID{src}, v1alpha1.EdgeOffshoot)
	offshoots.Delete(src)
	return g.links(oid, offshoots.UnsortedList(), edgeLabel)
}

func (g *ObjectGraph) links(oid *apiv1.ObjectID, seeds []apiv1.OID, edgeLabel v1alpha1.EdgeLabel) (map[metav1.GroupKind][]apiv1.ObjectID, error) {
	src := oid.OID()
	links := g.connectedOIDs(append([]apiv1.OID{src}, seeds...), edgeLabel)
	links.Delete(src)

	result := map[metav1.GroupKind][]apiv1.ObjectID{}
	for v := range links {
		id, err := apiv1.ParseObjectID(v)
		if err != nil {
			return nil, err
		}
		gk := id.MetaGroupKind()
		result[gk] = append(result[gk], *id)
	}
	return result, nil
}

func (g *ObjectGraph) connectedOIDs(idsToProcess []apiv1.OID, edgeLabel v1alpha1.EdgeLabel) setx.OID {
	links := setx.NewOID()
	var x apiv1.OID
	for len(idsToProcess) > 0 {
		x, idsToProcess = idsToProcess[0], idsToProcess[1:]
		links.Insert(x)

		var edges setx.OID
		if edgedPerLabel, ok := g.edges[x]; ok {
			edges = edgedPerLabel[edgeLabel]
		}
		for id := range edges {
			if !links.Has(id) {
				idsToProcess = append(idsToProcess, id)
			}
		}
	}
	return links
}
