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

	apiv1 "kmodules.xyz/client-go/api/v1"
)

// sets.OID is a set of apiv1.OIDs, implemented via map[apiv1.OID]struct{} for minimal memory consumption.
type OID map[apiv1.OID]Empty

// NewOID creates a OID from a list of values.
func NewOID(items ...apiv1.OID) OID {
	ss := make(OID, len(items))
	ss.Insert(items...)
	return ss
}

// OIDKeySet creates a OID from a keys of a map[apiv1.OID](? extends interface{}).
// If the value passed in is not actually a map, this will panic.
func OIDKeySet(theMap interface{}) OID {
	v := reflect.ValueOf(theMap)
	ret := OID{}

	for _, keyValue := range v.MapKeys() {
		ret.Insert(keyValue.Interface().(apiv1.OID))
	}
	return ret
}

// Insert adds items to the set.
func (s OID) Insert(items ...apiv1.OID) OID {
	for _, item := range items {
		s[item] = Empty{}
	}
	return s
}

// Delete removes all items from the set.
func (s OID) Delete(items ...apiv1.OID) OID {
	for _, item := range items {
		delete(s, item)
	}
	return s
}

// Has returns true if and only if item is contained in the set.
func (s OID) Has(item apiv1.OID) bool {
	_, contained := s[item]
	return contained
}

// HasAll returns true if and only if all items are contained in the set.
func (s OID) HasAll(items ...apiv1.OID) bool {
	for _, item := range items {
		if !s.Has(item) {
			return false
		}
	}
	return true
}

// HasAny returns true if any items are contained in the set.
func (s OID) HasAny(items ...apiv1.OID) bool {
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
func (s OID) Difference(s2 OID) OID {
	result := NewOID()
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
func (s1 OID) Union(s2 OID) OID {
	result := NewOID()
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
func (s1 OID) Intersection(s2 OID) OID {
	var walk, other OID
	result := NewOID()
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
func (s1 OID) IsSuperset(s2 OID) bool {
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
func (s1 OID) Equal(s2 OID) bool {
	return len(s1) == len(s2) && s1.IsSuperset(s2)
}

type sortableSliceOfOID []apiv1.OID

func (s sortableSliceOfOID) Len() int           { return len(s) }
func (s sortableSliceOfOID) Less(i, j int) bool { return lessOID(s[i], s[j]) }
func (s sortableSliceOfOID) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// List returns the contents as a sorted apiv1.OID slice.
func (s OID) List() []apiv1.OID {
	res := make(sortableSliceOfOID, 0, len(s))
	for key := range s {
		res = append(res, key)
	}
	sort.Sort(res)
	return []apiv1.OID(res)
}

// UnsortedList returns the slice with contents in random order.
func (s OID) UnsortedList() []apiv1.OID {
	res := make([]apiv1.OID, 0, len(s))
	for key := range s {
		res = append(res, key)
	}
	return res
}

// Returns a single element from the set.
func (s OID) PopAny() (apiv1.OID, bool) {
	for key := range s {
		s.Delete(key)
		return key, true
	}
	var zeroValue apiv1.OID
	return zeroValue, false
}

// Len returns the size of the set.
func (s OID) Len() int {
	return len(s)
}

func lessOID(lhs, rhs apiv1.OID) bool {
	return lhs < rhs
}
