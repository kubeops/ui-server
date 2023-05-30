/*
Copyright AppsCode Inc. and Contributors

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

package sets

import (
	"reflect"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// sets.MetaGroupVersionKind is a set of metav1.GroupVersionKinds, implemented via map[metav1.GroupVersionKind]struct{} for minimal memory consumption.
type MetaGroupVersionKind map[metav1.GroupVersionKind]Empty

// NewMetaGroupVersionKind creates a MetaGroupVersionKind from a list of values.
func NewMetaGroupVersionKind(items ...metav1.GroupVersionKind) MetaGroupVersionKind {
	ss := make(MetaGroupVersionKind, len(items))
	ss.Insert(items...)
	return ss
}

// MetaGroupVersionKindKeySet creates a MetaGroupVersionKind from a keys of a map[metav1.GroupVersionKind](? extends interface{}).
// If the value passed in is not actually a map, this will panic.
func MetaGroupVersionKindKeySet(theMap interface{}) MetaGroupVersionKind {
	v := reflect.ValueOf(theMap)
	ret := MetaGroupVersionKind{}

	for _, keyValue := range v.MapKeys() {
		ret.Insert(keyValue.Interface().(metav1.GroupVersionKind))
	}
	return ret
}

// Insert adds items to the set.
func (s MetaGroupVersionKind) Insert(items ...metav1.GroupVersionKind) MetaGroupVersionKind {
	for _, item := range items {
		s[item] = Empty{}
	}
	return s
}

// Delete removes all items from the set.
func (s MetaGroupVersionKind) Delete(items ...metav1.GroupVersionKind) MetaGroupVersionKind {
	for _, item := range items {
		delete(s, item)
	}
	return s
}

// Has returns true if and only if item is contained in the set.
func (s MetaGroupVersionKind) Has(item metav1.GroupVersionKind) bool {
	_, contained := s[item]
	return contained
}

// HasAll returns true if and only if all items are contained in the set.
func (s MetaGroupVersionKind) HasAll(items ...metav1.GroupVersionKind) bool {
	for _, item := range items {
		if !s.Has(item) {
			return false
		}
	}
	return true
}

// HasAny returns true if any items are contained in the set.
func (s MetaGroupVersionKind) HasAny(items ...metav1.GroupVersionKind) bool {
	for _, item := range items {
		if s.Has(item) {
			return true
		}
	}
	return false
}

// Difference returns a set of objects that are not in s2
// For example:
// s1 = {a1, a2, a3}
// s2 = {a1, a2, a4, a5}
// s1.Difference(s2) = {a3}
// s2.Difference(s1) = {a4, a5}
func (s MetaGroupVersionKind) Difference(s2 MetaGroupVersionKind) MetaGroupVersionKind {
	result := NewMetaGroupVersionKind()
	for key := range s {
		if !s2.Has(key) {
			result.Insert(key)
		}
	}
	return result
}

// Union returns a new set which includes items in either s1 or s2.
// For example:
// s1 = {a1, a2}
// s2 = {a3, a4}
// s1.Union(s2) = {a1, a2, a3, a4}
// s2.Union(s1) = {a1, a2, a3, a4}
func (s1 MetaGroupVersionKind) Union(s2 MetaGroupVersionKind) MetaGroupVersionKind {
	result := NewMetaGroupVersionKind()
	for key := range s1 {
		result.Insert(key)
	}
	for key := range s2 {
		result.Insert(key)
	}
	return result
}

// Intersection returns a new set which includes the item in BOTH s1 and s2
// For example:
// s1 = {a1, a2}
// s2 = {a2, a3}
// s1.Intersection(s2) = {a2}
func (s1 MetaGroupVersionKind) Intersection(s2 MetaGroupVersionKind) MetaGroupVersionKind {
	var walk, other MetaGroupVersionKind
	result := NewMetaGroupVersionKind()
	if s1.Len() < s2.Len() {
		walk = s1
		other = s2
	} else {
		walk = s2
		other = s1
	}
	for key := range walk {
		if other.Has(key) {
			result.Insert(key)
		}
	}
	return result
}

// IsSuperset returns true if and only if s1 is a superset of s2.
func (s1 MetaGroupVersionKind) IsSuperset(s2 MetaGroupVersionKind) bool {
	for item := range s2 {
		if !s1.Has(item) {
			return false
		}
	}
	return true
}

// Equal returns true if and only if s1 is equal (as a set) to s2.
// Two sets are equal if their membership is identical.
// (In practice, this means same elements, order doesn't matter)
func (s1 MetaGroupVersionKind) Equal(s2 MetaGroupVersionKind) bool {
	return len(s1) == len(s2) && s1.IsSuperset(s2)
}

type sortableSliceOfMetaGroupVersionKind []metav1.GroupVersionKind

func (s sortableSliceOfMetaGroupVersionKind) Len() int           { return len(s) }
func (s sortableSliceOfMetaGroupVersionKind) Less(i, j int) bool { return lessMetaGroupVersionKind(s[i], s[j]) }
func (s sortableSliceOfMetaGroupVersionKind) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// List returns the contents as a sorted metav1.GroupVersionKind slice.
func (s MetaGroupVersionKind) List() []metav1.GroupVersionKind {
	res := make(sortableSliceOfMetaGroupVersionKind, 0, len(s))
	for key := range s {
		res = append(res, key)
	}
	sort.Sort(res)
	return []metav1.GroupVersionKind(res)
}

// UnsortedList returns the slice with contents in random order.
func (s MetaGroupVersionKind) UnsortedList() []metav1.GroupVersionKind {
	res := make([]metav1.GroupVersionKind, 0, len(s))
	for key := range s {
		res = append(res, key)
	}
	return res
}

// Returns a single element from the set.
func (s MetaGroupVersionKind) PopAny() (metav1.GroupVersionKind, bool) {
	for key := range s {
		s.Delete(key)
		return key, true
	}
	var zeroValue metav1.GroupVersionKind
	return zeroValue, false
}

// Len returns the size of the set.
func (s MetaGroupVersionKind) Len() int {
	return len(s)
}

func lessMetaGroupVersionKind(lhs, rhs metav1.GroupVersionKind) bool {
	if lhs.Group != rhs.Group {
		return lhs.Group < rhs.Group
	}
	return lhs.Kind < rhs.Kind
}
