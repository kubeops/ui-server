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

package graph

import (
	"testing"
)

func TestExtractName(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		result   string
	}{
		{
			name:     "kubedb-sample",
			selector: "kubedb-{.metadata.name}",
			result:   "sample",
		},
		{
			name:     "sample-kubedb",
			selector: "{.metadata.name}-kubedb",
			result:   "sample",
		},
		{
			name:     "my.db~Elasticsearch.kubedb.com",
			selector: "{.metadata.name}~Elasticsearch.kubedb.com",
			result:   "my.db",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r, ok := ExtractName(test.name, test.selector)
			if !ok {
				t.FailNow()
			}
			if test.result != r {
				t.FailNow()
			}
		})
	}
}
