/*

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

package watch

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type vitals struct {
	gvk        schema.GroupVersionKind
	registrars map[*Registrar]bool
}

type vitalsByGVK map[schema.GroupVersionKind]vitals

func (w *vitals) merge(wv vitals) vitals {
	if w == nil {
		return wv
	}
	registrars := make(map[*Registrar]bool)
	for r := range w.registrars {
		registrars[r] = true
	}
	for r := range wv.registrars {
		registrars[r] = true
	}
	return vitals{
		gvk:        w.gvk,
		registrars: registrars,
	}
}

// recordKeeper holds the source of truth for the intended state of the manager
// This is essentially a read/write lock on the wrapped map (the `intent` variable).
type recordKeeper struct {
	// map[registrarName][kind]
	intent     map[string]vitalsByGVK
	intentMux  sync.RWMutex
	registrars map[string]*Registrar
	mgr        *Manager
	metrics    *reporter
}

func (r *recordKeeper) NewRegistrar(parentName string, events chan<- event.GenericEvent) (*Registrar, error) {
	r.intentMux.Lock()
	defer r.intentMux.Unlock()
	if _, ok := r.registrars[parentName]; ok {
		return nil, fmt.Errorf("registrar for %s already exists", parentName)
	}
	out := &Registrar{
		parentName:   parentName,
		mgr:          r.mgr,
		managedKinds: r,
		events:       events,
	}
	r.registrars[parentName] = out
	return out, nil
}

// RemoveRegistrar removes a registrar and all its watches.
func (r *recordKeeper) RemoveRegistrar(parentName string) error {
	r.intentMux.Lock()
	registrar := r.registrars[parentName]
	r.intentMux.Unlock()

	if registrar == nil {
		return nil
	}
	if err := registrar.ReplaceWatch(context.Background(), nil); err != nil {
		return err
	}

	r.intentMux.Lock()
	defer r.intentMux.Unlock()
	delete(r.registrars, parentName)
	return nil
}

func (r *recordKeeper) Update(parentName string, m vitalsByGVK) {
	r.intentMux.Lock()
	defer r.intentMux.Unlock()

	defer func() {
		if err := r.metrics.reportGvkIntentCount(int64(r.count())); err != nil {
			log.Error(err, "while reporting gvk intent count metric")
		}
	}()

	if _, ok := r.intent[parentName]; !ok {
		r.intent[parentName] = make(vitalsByGVK)
	}
	for gvk, v := range m {
		r.intent[parentName][gvk] = v
	}
}

// ReplaceRegistrarRoster replaces the desired set of watches for the specified registrar using provided roster.
// Ownership is taken over roster - it is not currently deep-copied.
func (r *recordKeeper) ReplaceRegistrarRoster(reg *Registrar, roster map[schema.GroupVersionKind]vitals) {
	r.intentMux.Lock()
	defer r.intentMux.Unlock()
	defer func() {
		if err := r.metrics.reportGvkIntentCount(int64(r.count())); err != nil {
			log.Error(err, "while reporting gvk intent count metric")
		}
	}()

	r.intent[reg.parentName] = roster
}

// Watching returns whether a GVK is being watched by a given registrar.
func (r *recordKeeper) Watching(parentName string, gvk schema.GroupVersionKind) bool {
	r.intentMux.RLock()
	defer r.intentMux.RUnlock()
	_, ok := r.intent[parentName][gvk]
	return ok
}

// Remove removes the intent-to-watch a particular resource kind.
func (r *recordKeeper) Remove(parentName string, gvk schema.GroupVersionKind) {
	r.intentMux.Lock()
	defer r.intentMux.Unlock()
	defer func() {
		if err := r.metrics.reportGvkIntentCount(int64(r.count())); err != nil {
			log.Error(err, "while reporting gvk intent count metric")
		}
	}()

	delete(r.intent[parentName], gvk)
}

// Get returns all managed vitals, merged across registrars.
func (r *recordKeeper) Get() vitalsByGVK {
	r.intentMux.RLock()
	defer r.intentMux.RUnlock()
	cpy := make(map[string]vitalsByGVK)
	for k := range r.intent {
		cpy[k] = make(vitalsByGVK)
		for k2, v := range r.intent[k] {
			cpy[k][k2] = v
		}
	}
	managedKinds := make(vitalsByGVK)
	for _, registrar := range cpy {
		for gvk, v := range registrar {
			if mk, ok := managedKinds[gvk]; ok {
				merged := mk.merge(v)
				managedKinds[gvk] = merged
			} else {
				managedKinds[gvk] = v
			}
		}
	}
	return managedKinds
}

// count returns total gvk count across all registrars.
func (r *recordKeeper) count() int {
	managedKinds := make(map[schema.GroupVersionKind]bool)
	for _, registrar := range r.intent {
		for gvk := range registrar {
			managedKinds[gvk] = true
		}
	}
	return len(managedKinds)
}

// GetGVK returns all managed kinds, merged across registrars.
func (r *recordKeeper) GetGVK() []schema.GroupVersionKind {
	var gvks []schema.GroupVersionKind

	g := r.Get()
	for gvk := range g {
		gvks = append(gvks, gvk)
	}

	sort.Slice(gvks, func(i, j int) bool {
		return gvks[i].String() < gvks[j].String()
	})
	return gvks
}

func newRecordKeeper() (*recordKeeper, error) {
	metrics, err := newStatsReporter()
	if err != nil {
		return nil, err
	}
	return &recordKeeper{
		intent:     make(map[string]vitalsByGVK),
		registrars: make(map[string]*Registrar),
		metrics:    metrics,
	}, nil
}

// A Registrar allows a parent to add/remove child watches.
type Registrar struct {
	parentName   string
	mgr          *Manager
	managedKinds *recordKeeper
	events       chan<- event.GenericEvent
	mux          sync.RWMutex
}

// AddWatch registers a watch for the given kind.
//
// AddWatch will only block if all of the following are true:
//   - The registrar is joining an existing watch
//   - The registrar's event channel does not have sufficient capacity to receive existing resources
//   - The consumer of the channel does not receive any unbuffered events.
//
// XXXX also may block if the watch manager has not been started.
func (r *Registrar) AddWatch(ctx context.Context, gvk schema.GroupVersionKind) error {
	r.mux.Lock()
	defer r.mux.Unlock()
	wv := vitals{
		gvk:        gvk,
		registrars: map[*Registrar]bool{r: true},
	}
	r.managedKinds.Update(r.parentName, vitalsByGVK{gvk: wv})
	return r.mgr.addWatch(ctx, r, gvk)
}

// ReplaceWatch replaces the set of watched resources.
func (r *Registrar) ReplaceWatch(ctx context.Context, gvks []schema.GroupVersionKind) error {
	r.mux.Lock()
	defer r.mux.Unlock()
	roster := make(vitalsByGVK)
	for _, gvk := range gvks {
		wv := vitals{
			gvk:        gvk,
			registrars: map[*Registrar]bool{r: true},
		}
		roster[gvk] = wv
	}
	r.managedKinds.ReplaceRegistrarRoster(r, roster)
	return r.mgr.replaceWatches(ctx, r)
}

// RemoveWatch removes a watch for the given kind.
// Ignores the request if the kind was not previously watched.
func (r *Registrar) RemoveWatch(ctx context.Context, gvk schema.GroupVersionKind) error {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.managedKinds.Remove(r.parentName, gvk)
	return r.mgr.removeWatch(ctx, r, gvk)
}

// IfWatching executes the passed function if the provided GVK is being watched
// by the registrar, ignoring it if not. It returns whether the function was
// executed and any errors returned by the executed function.
func (r *Registrar) IfWatching(gvk schema.GroupVersionKind, fn func() error) (bool, error) {
	r.mux.RLock()
	defer r.mux.RUnlock()
	if r.managedKinds.Watching(r.parentName, gvk) {
		return true, fn()
	}
	return false, nil
}
