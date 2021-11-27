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

package shared

import (
	"testing"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGroupKindSelector_Matches(t *testing.T) {
	podGK := schema.GroupKind{Group: "", Kind: "Pod"}
	svcGK := schema.GroupKind{Group: "", Kind: "Service"}

	tests := []struct {
		name   string
		fields labels.Selector
		args   schema.GroupKind
		want   bool
	}{
		{
			name:   "Nil Selector",
			fields: nil,
			args:   podGK,
			want:   true,
		},
		{
			name:   "Empty Selector",
			fields: labels.Everything(),
			args:   podGK,
			want:   true,
		},
		{
			name:   "Nothing Selector",
			fields: labels.Nothing(),
			args:   podGK,
			want:   false,
		},
		{
			name: "Group Selector",
			fields: labels.SelectorFromSet(map[string]string{
				"k8s.io/group": "",
			}),
			args: podGK,
			want: true,
		},
		{
			name: "Group:Pod Selector",
			fields: labels.SelectorFromSet(map[string]string{
				"k8s.io/group":      "",
				"k8s.io/group-kind": svcGK.String(),
			}),
			args: podGK,
			want: false,
		},
		{
			name: "Group:Service Selector",
			fields: labels.SelectorFromSet(map[string]string{
				"k8s.io/group":      "",
				"k8s.io/group-kind": svcGK.String(),
			}),
			args: svcGK,
			want: true,
		},
		{
			name: "GroupKind:Pod Selector",
			fields: labels.SelectorFromSet(map[string]string{
				"k8s.io/group-kind": podGK.String(),
			}),
			args: podGK,
			want: true,
		},
		{
			name: "GroupKind:Service Selector",
			fields: labels.SelectorFromSet(map[string]string{
				"k8s.io/group-kind": podGK.String(),
			}),
			args: svcGK,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewGroupKindSelector(tt.fields)
			if got := s.Matches(tt.args); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}
