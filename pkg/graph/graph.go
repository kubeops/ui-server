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
	"encoding/json"
	"sync"

	"gomodules.xyz/sets"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/hub"
	ksets "kmodules.xyz/sets"
)

type ObjectGraph struct {
	m sync.RWMutex `json:"-"`
	// bool == true , self link
	Edges map[kmapi.OID]map[kmapi.EdgeLabel]map[kmapi.OID]bool `json:"edges,omitempty"` // oid -> label -> Edges
	IDs   map[kmapi.OID]map[kmapi.EdgeLabel]ksets.OID          `json:"ids,omitempty"`   // oid -> label -> Edges
}

func (g *ObjectGraph) render(src kmapi.OID) (*runtime.RawExtension, error) {
	if src == "" {
		data, err := json.Marshal(g)
		if err != nil {
			return nil, err
		}
		return &runtime.RawExtension{
			Raw: data,
		}, nil
	}

	srcGraph := struct {
		Edges map[kmapi.EdgeLabel]map[kmapi.OID]bool `json:"edges,omitempty"` // oid -> label -> Edges
		IDs   map[kmapi.EdgeLabel]ksets.OID          `json:"ids,omitempty"`   // oid -> label -> Edges
	}{
		Edges: g.Edges[src],
		IDs:   g.IDs[src],
	}
	data, err := json.Marshal(srcGraph)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{
		Raw: data,
	}, nil
}

func (g *ObjectGraph) Update(src kmapi.OID, connsPerLabel map[kmapi.EdgeLabel]ksets.OID) {
	g.m.Lock()
	defer g.m.Unlock()

	for lbl, connMap := range g.Edges[src] {
		for to, srcLink := range connMap {
			if srcLink {
				delete(connMap, to) // reassign map?
				// delete(g.Edges[to][lbl], src)
				g.delEdge(to, src, lbl)
			}
		}
	}
	for lbl, conns := range connsPerLabel {
		for to := range conns {
			g.setEdge(src, to, lbl, true)
			g.setEdge(to, src, lbl, false)
		}
	}

	if len(connsPerLabel) == 0 {
		delete(g.IDs, src)
	} else {
		g.IDs[src] = connsPerLabel
	}
}

func (g *ObjectGraph) setEdge(src, to kmapi.OID, lbl kmapi.EdgeLabel, self bool) {
	if g.Edges == nil {
		g.Edges = map[kmapi.OID]map[kmapi.EdgeLabel]map[kmapi.OID]bool{}
	}
	if g.Edges[src] == nil {
		g.Edges[src] = map[kmapi.EdgeLabel]map[kmapi.OID]bool{}
	}
	if g.Edges[src][lbl] == nil {
		g.Edges[src][lbl] = map[kmapi.OID]bool{}
	}
	g.Edges[src][lbl][to] = self
}

func (g *ObjectGraph) delEdge(src, to kmapi.OID, lbl kmapi.EdgeLabel) {
	if _, ok := g.Edges[src]; !ok {
		return
	}
	if _, ok := g.Edges[src][lbl]; !ok {
		return
	}
	delete(g.Edges[src][lbl], to)
}

func (g *ObjectGraph) Links(oid *kmapi.ObjectID, edgeLabel kmapi.EdgeLabel) (map[metav1.GroupKind][]kmapi.ObjectID, error) {
	g.m.RLock()
	defer g.m.RUnlock()

	if edgeLabel.Direct() {
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
	result := ksets.NewOID()
	var x kmapi.OID
	for len(idsToProcess) > 0 {
		x, idsToProcess = idsToProcess[0], idsToProcess[1:]
		processed.Insert(x)

		edges := ksets.NewOID()
		if edgedPerLabel, ok := g.Edges[x]; ok {
			for to := range edgedPerLabel[edgeLabel] {
				edges.Insert(to)
			}
		}
		result = result.Union(edges)

		for id := range edges {
			if !processed.Has(id) {
				idsToProcess = append(idsToProcess, id)
			}
		}
	}
	return result
}

type objectEdge struct {
	Source kmapi.OID
	Target kmapi.OID
}

func Render(src kmapi.OID) (*runtime.RawExtension, error) {
	objGraph.m.RLock()
	defer objGraph.m.RUnlock()

	return objGraph.render(src)
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

		edges := ksets.NewOID()
		if edgedPerLabel, ok := g.Edges[x]; ok {
			for to := range edgedPerLabel[edgeLabel] {
				edges.Insert(to)
			}
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
