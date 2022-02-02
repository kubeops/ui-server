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

	"gomodules.xyz/sets"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub"
	ksets "kmodules.xyz/sets"
)

type ObjectGraph struct {
	m     sync.RWMutex
	edges map[kmapi.OID]map[kmapi.EdgeLabel]ksets.OID // oid -> label -> edges
	ids   map[kmapi.OID]map[kmapi.EdgeLabel]ksets.OID // oid -> label -> edges
}

func (g *ObjectGraph) Update(src kmapi.OID, connsPerLabel map[kmapi.EdgeLabel]ksets.OID) {
	g.m.Lock()
	defer g.m.Unlock()

	for lbl, conns := range connsPerLabel {

		if oldConnsPerLabel, ok := g.ids[src]; ok {
			if oldConns, ok := oldConnsPerLabel[lbl]; ok {
				if oldConns.Difference(conns).Len() == 0 {
					continue
				}

				g.edges[src][lbl].Delete(oldConns.UnsortedList()...)
				for dst := range oldConns {
					g.edges[dst][lbl].Delete(src)
				}
			}
		}

		if _, ok := g.edges[src]; !ok {
			g.edges[src] = map[kmapi.EdgeLabel]ksets.OID{}
		}
		if _, ok := g.edges[src][lbl]; !ok {
			g.edges[src][lbl] = ksets.NewOID()
		}
		g.edges[src][lbl].Insert(conns.UnsortedList()...)

		for dst := range conns {
			if _, ok := g.edges[dst]; !ok {
				g.edges[dst] = map[kmapi.EdgeLabel]ksets.OID{}
			}
			if _, ok := g.edges[dst][lbl]; !ok {
				g.edges[dst][lbl] = ksets.NewOID()
			}
			g.edges[dst][lbl].Insert(src)
		}
	}

	// remove edged that don't exist anymore
	oldConnsPerLabel := g.ids[src]
	for lbl, conns := range oldConnsPerLabel {
		if _, ok := connsPerLabel[lbl]; ok {
			continue
		}

		g.edges[src][lbl].Delete(conns.UnsortedList()...)
		for dst := range conns {
			g.edges[dst][lbl].Delete(src)
		}
	}

	if len(connsPerLabel) == 0 {
		delete(g.ids, src)
	} else {
		g.ids[src] = connsPerLabel
	}
}

func (g *ObjectGraph) Links(oid *kmapi.ObjectID, edgeLabel kmapi.EdgeLabel) (map[metav1.GroupKind][]kmapi.ObjectID, error) {
	g.m.RLock()
	defer g.m.RUnlock()

	if edgeLabel == kmapi.EdgeOffshoot || edgeLabel == kmapi.EdgeView {
		return g.links(oid, nil, edgeLabel)
	}

	src := oid.OID()
	offshoots := g.connectedOIDs([]kmapi.OID{src}, kmapi.EdgeOffshoot)
	offshoots.Delete(src)
	return g.links(oid, offshoots.UnsortedList(), edgeLabel)
}

func (g *ObjectGraph) links(oid *kmapi.ObjectID, seeds []kmapi.OID, edgeLabel kmapi.EdgeLabel) (map[metav1.GroupKind][]kmapi.ObjectID, error) {
	src := oid.OID()
	links := g.connectedOIDs(append([]kmapi.OID{src}, seeds...), edgeLabel)
	links.Delete(src)

	result := map[metav1.GroupKind][]kmapi.ObjectID{}
	for v := range links {
		id, err := kmapi.ParseObjectID(v)
		if err != nil {
			return nil, err
		}
		gk := id.MetaGroupKind()
		result[gk] = append(result[gk], *id)
	}
	return result, nil
}

func (g *ObjectGraph) connectedOIDs(idsToProcess []kmapi.OID, edgeLabel kmapi.EdgeLabel) ksets.OID {
	processed := ksets.NewOID()
	var x kmapi.OID
	for len(idsToProcess) > 0 {
		x, idsToProcess = idsToProcess[0], idsToProcess[1:]
		processed.Insert(x)

		var edges ksets.OID
		if edgedPerLabel, ok := g.edges[x]; ok {
			edges = edgedPerLabel[edgeLabel]
		}
		for id := range edges {
			if !processed.Has(id) {
				idsToProcess = append(idsToProcess, id)
			}
		}
	}
	return processed
}

type objectEdge struct {
	Source kmapi.OID
	Target kmapi.OID
}

func ResourceGraph(mapper meta.RESTMapper, src kmapi.ObjectID) (*rsapi.ResourceGraphResponse, error) {
	objGraph.m.RLock()
	defer objGraph.m.RUnlock()

	return objGraph.resourceGraph(mapper, src)
}

func (g *ObjectGraph) resourceGraph(mapper meta.RESTMapper, src kmapi.ObjectID) (*rsapi.ResourceGraphResponse, error) {
	connections := map[objectEdge]sets.String{}

	offshoots := g.connectedEdges([]kmapi.OID{src.OID()}, kmapi.EdgeOffshoot, ksets.NewGroupKind(), connections).UnsortedList()
	skipGKs := ksets.NewGroupKind()
	var objID *kmapi.ObjectID
	for _, oid := range offshoots {
		objID, _ = kmapi.ParseObjectID(oid)
		skipGKs.Insert(objID.GroupKind())
	}
	for _, label := range hub.ListEdgeLabels(kmapi.EdgeOffshoot, kmapi.EdgeView) {
		g.connectedEdges(offshoots, label, skipGKs, connections)
	}

	gkSet := ksets.NewGroupKind()
	for e := range connections {
		objID, _ = kmapi.ParseObjectID(e.Source)
		gkSet.Insert(objID.GroupKind())
		objID, _ = kmapi.ParseObjectID(e.Target)
		gkSet.Insert(objID.GroupKind())
	}
	gks := gkSet.List()

	resp := rsapi.ResourceGraphResponse{
		Resources:   make([]kmapi.ResourceID, len(gks)),
		Connections: make([]rsapi.ObjectConnection, 0, len(connections)),
	}

	gkMap := map[schema.GroupKind]int{}
	for idx, gk := range gks {
		gkMap[gk] = idx

		mapping, err := mapper.RESTMapping(gk)
		if err != nil {
			return nil, err
		}
		resp.Resources[idx] = *kmapi.NewResourceID(mapping)
	}

	for e, labels := range connections {
		src, _ := kmapi.ParseObjectID(e.Source)
		target, _ := kmapi.ParseObjectID(e.Target)

		resp.Connections = append(resp.Connections, rsapi.ObjectConnection{
			Source: rsapi.ObjectPointer{
				ResourceID: gkMap[src.GroupKind()],
				Namespace:  src.Namespace,
				Name:       src.Name,
			},
			Target: rsapi.ObjectPointer{
				ResourceID: gkMap[target.GroupKind()],
				Namespace:  target.Namespace,
				Name:       target.Name,
			},
			Labels: labels.List(),
		})
	}
	return &resp, nil
}

func (g *ObjectGraph) connectedEdges(idsToProcess []kmapi.OID, edgeLabel kmapi.EdgeLabel, skipGKs ksets.GroupKind, connections map[objectEdge]sets.String) ksets.OID {
	processed := ksets.NewOID()
	var x kmapi.OID
	var objID *kmapi.ObjectID
	for len(idsToProcess) > 0 {
		x, idsToProcess = idsToProcess[0], idsToProcess[1:]
		processed.Insert(x)

		var edges ksets.OID
		if edgedPerLabel, ok := g.edges[x]; ok {
			edges = edgedPerLabel[edgeLabel]
		}
		for id := range edges {
			objID, _ = kmapi.ParseObjectID(id)
			if skipGKs.Len() == 0 || !skipGKs.Has(objID.GroupKind()) {
				var key objectEdge
				if x < id {
					key = objectEdge{
						Source: x,
						Target: id,
					}
				} else {
					key = objectEdge{
						Source: id,
						Target: x,
					}
				}
				if _, ok := connections[key]; !ok {
					connections[key] = sets.NewString()
				}
				connections[key].Insert(string(edgeLabel))

				if !processed.Has(id) {
					idsToProcess = append(idsToProcess, id)
				}
			}
		}
	}
	return processed
}
