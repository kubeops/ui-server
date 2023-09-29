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
	"context"
	"net/http"
	"time"

	"kubeops.dev/ui-server/pkg/graph"
	"kubeops.dev/ui-server/pkg/metricsstore"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apiserver/pkg/server/mux"
	"k8s.io/klog/v2"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	MetricsPath         = "/metrics"
	scannerMetricPrefix = "scanner_appscode_com_"
	policyMetricPrefix  = "policy_appscode_com_"
)

// MetricsHandler struct contains Stores which store the metrics to serve in the /metrics path
type MetricsHandler struct {
	client.Client
}

var (
	generators       []generator.FamilyGenerator
	store            *metricsstore.MetricsStore
	opaInstalled     bool
	scannerInstalled bool
)

// ServeHTTP serves the request for /metrics path
func (m *MetricsHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	resHeader := w.Header()
	resHeader.Set("Content-Type", `text/plain; version=`+"0.0.4")
	err := store.WriteAll(w)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// Install adds the MetricsWithReset handler
func (m *MetricsHandler) Install(c *mux.PathRecorderMux) {
	var next http.Handler = m
	next = promhttp.InstrumentHandlerCounter(httpRequestsTotal, next)
	next = promhttp.InstrumentHandlerDuration(requestDuration, next)
	next = promhttp.InstrumentHandlerInFlight(inFlight, next)
	next = promhttp.InstrumentHandlerRequestSize(requestSize, next)
	next = promhttp.InstrumentHandlerResponseSize(responseSize, next)
	c.Handle(MetricsPath, next)
}

func StartMetricsCollector(mgr manager.Manager) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		go startMetricsCollector(mgr.GetClient())
		return nil
	}
}

func startMetricsCollector(kc client.Client) {
	klog.Infoln("Starts the Metrics Collector")
	t := time.NewTicker(time.Minute * 2)
	defer t.Stop()

	for {
		scannerInstalled = graph.ScannerInstalled()
		opaInstalled = graph.OPAInstalled()
		generators = getFamilyGenerators()
		headers := generator.ExtractMetricFamilyHeaders(generators)
		store = metricsstore.NewMetricsStore(headers)
		err := collectMetrics(kc)
		if err != nil {
			klog.Errorf("Error occurred while collecting metrics : %s \n", err.Error())
		}
		<-t.C
	}
}

func collectMetrics(kc client.Client) error {
	err := collectPodAncestorMetrics(kc, generators, store)
	if err != nil {
		return err
	}

	offset := 1
	if scannerInstalled {
		err := collectScannerMetrics(kc, generators, store, offset)
		if err != nil {
			return err
		}
		offset = offset + 9 // # of scanner metrics families
	}
	if opaInstalled {
		err := collectPolicyMetrics(kc, generators, store, offset)
		if err != nil {
			return err
		}
	}

	return nil
}

func getFamilyGenerators() []generator.FamilyGenerator {
	fn := func(obj interface{}) *metric.Family { return new(metric.Family) }
	generators = make([]generator.FamilyGenerator, 0, 14)

	generators = append(generators, generator.FamilyGenerator{
		Name:              "k8s_appscode_com_pod_ancestor",
		Help:              "Pod Ancestor statistics",
		Type:              metric.Gauge,
		DeprecatedVersion: "",
		GenerateFunc:      fn,
	})

	if scannerInstalled {
		generators = append(generators, generator.FamilyGenerator{
			Name:              scannerMetricPrefix + "cluster_cve_occurrence",
			Help:              "CVE occurrence statistics",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
		generators = append(generators, generator.FamilyGenerator{
			Name:              scannerMetricPrefix + "cluster_cve_occurrence_total",
			Help:              "Cluster total CVE occurrence",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
		generators = append(generators, generator.FamilyGenerator{
			Name:              scannerMetricPrefix + "cluster_cve_count_total",
			Help:              "Cluster total unique CVE count",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
		generators = append(generators, generator.FamilyGenerator{
			Name:              scannerMetricPrefix + "namespace_cve_occurrence",
			Help:              "Namespace CVE occurrence statistics",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
		generators = append(generators, generator.FamilyGenerator{
			Name:              scannerMetricPrefix + "namespace_cve_occurrence_total",
			Help:              "Namespace total CVE occurrence",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
		generators = append(generators, generator.FamilyGenerator{
			Name:              scannerMetricPrefix + "namespace_cve_count_total",
			Help:              "Namespace total unique CVE count",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})

		generators = append(generators, generator.FamilyGenerator{
			Name:              scannerMetricPrefix + "image_cve_occurrence_total",
			Help:              "Image total CVE occurrence",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
		generators = append(generators, generator.FamilyGenerator{
			Name:              scannerMetricPrefix + "image_cve_count_total",
			Help:              "Image total unique CVE count",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
		generators = append(generators, generator.FamilyGenerator{
			Name:              scannerMetricPrefix + "image_lineage",
			Help:              "Image Lineage",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
	}

	if opaInstalled {
		// Policy related metrics
		generators = append(generators, generator.FamilyGenerator{
			Name:              policyMetricPrefix + "cluster_violation_occurrence_total",
			Help:              "Cluster-wide Violation Occurrence statistics",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
		generators = append(generators, generator.FamilyGenerator{
			Name:              policyMetricPrefix + "cluster_violation_occurrence_by_constraint_type",
			Help:              "Cluster-wide Violation Occurrence statistics by constraint type",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
		generators = append(generators, generator.FamilyGenerator{
			Name:              policyMetricPrefix + "namespace_violation_occurrence_total",
			Help:              "Namespace-wise total Violation Occurrence statistics",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
		generators = append(generators, generator.FamilyGenerator{
			Name:              policyMetricPrefix + "namespace_violation_occurrence_by_constraint_type",
			Help:              "Namespace-wise Violation Occurrence statistics by constraint type",
			Type:              metric.Gauge,
			DeprecatedVersion: "",
			GenerateFunc:      fn,
		})
	}
	return generators
}
