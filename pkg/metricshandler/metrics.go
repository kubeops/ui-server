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

package metricshandler

import (
	"github.com/prometheus/client_golang/prometheus"
	apimetrics "k8s.io/apiserver/pkg/endpoints/metrics"
	cachermetrics "k8s.io/apiserver/pkg/storage/cacher/metrics"
	etcd3metrics "k8s.io/apiserver/pkg/storage/etcd3/metrics"
	flowcontrolmetrics "k8s.io/apiserver/pkg/util/flowcontrol/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Count of all HTTP requests for metrics handler",
			ConstLabels: prometheus.Labels{
				"handler": "metrics",
			},
		},
		[]string{"code", "method"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "A histogram of requests for metrics handler.",
			Buckets: prometheus.DefBuckets,
			ConstLabels: prometheus.Labels{
				"handler": "metrics",
			},
		},
		[]string{"code", "method"},
	)
	inFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Current number of requests being served by metrics handler",
			ConstLabels: prometheus.Labels{
				"handler": "metrics",
			},
		},
	)
	requestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_size_bytes",
			Help:    "Histogram of HTTP request size for metrics handler",
			Buckets: prometheus.ExponentialBuckets(100, 10, 7),
			ConstLabels: prometheus.Labels{
				"handler": "metrics",
			},
		},
		[]string{"code", "method"},
	)
	responseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_response_size_bytes",
			Help:    "Histogram of response size for HTTP requests served by metrics handler",
			Buckets: prometheus.ExponentialBuckets(100, 10, 7),
			ConstLabels: prometheus.Labels{
				"handler": "metrics",
			},
		},
		[]string{"code", "method"},
	)
)

func RegisterSelfMetrics() {
	legacyregistry.RawMustRegister(httpRequestsTotal)
	legacyregistry.RawMustRegister(requestDuration)
	legacyregistry.RawMustRegister(inFlight)
	legacyregistry.RawMustRegister(requestSize)
	legacyregistry.RawMustRegister(responseSize)

	// ref: https://github.com/kubernetes/apiserver/blob/v0.25.3/pkg/server/routes/metrics.go#L47-L53
	apimetrics.Register()
	cachermetrics.Register()
	etcd3metrics.Register()
	flowcontrolmetrics.Register()
}
