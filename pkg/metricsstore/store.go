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

package metricsstore

import (
	"io"

	"k8s.io/kube-state-metrics/v2/pkg/metric"
)

// MetricsStore stores metrics for a single scrape by a Prometheus server.
type MetricsStore struct {
	// headers contains the header (TYPE and HELP) of each metric family. It is
	// later on zipped with with their corresponding metric families in
	// MetricStore.WriteAll().
	headers []string
	// Families is a slice of metric families, containing a slice of metrics.
	// We need to keep metrics grouped by metric families in order to
	// zip families with their help text in  MetricsStore.WriteAll().
	families []metric.Family
}

// NewMetricsStore returns a new MetricsStore
func NewMetricsStore(headers []string) *MetricsStore {
	return &MetricsStore{
		headers:  headers,
		families: make([]metric.Family, 0, len(headers)),
	}
}

func (s *MetricsStore) Add(family ...*metric.Family) {
	for _, f := range family {
		s.families = append(s.families, *f)
	}
}

// WriteAll writes all metrics of the store into the given writer, zipped with the
// help text of each metric family.
func (s *MetricsStore) WriteAll(w io.Writer) error {
	for i, help := range s.headers {
		_, err := w.Write([]byte(help))
		if err != nil {
			return err
		}
		_, err = w.Write([]byte{'\n'})
		if err != nil {
			return err
		}
		_, err = w.Write(s.families[i].ByteSlice())
		if err != nil {
			return err
		}
	}
	return nil
}
