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

	scannerapi "kubeops.dev/scanner/apis/scanner/v1alpha1"
	"kubeops.dev/ui-server/pkg/shared"

	"golang.org/x/sync/errgroup"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
	kmapi "kmodules.xyz/client-go/api/v1"
	au "kmodules.xyz/client-go/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (mc *Collector) collectScannerMetrics(offset int) error {
	var list unstructured.UnstructuredList
	list.SetAPIVersion("v1")
	list.SetKind("Pod")
	if err := mc.kc.List(context.TODO(), &list); err != nil {
		return err
	}
	pods := list.Items

	images := map[string]kmapi.ImageInfo{}
	var err error
	for _, p := range pods {
		var pod core.Pod
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(p.UnstructuredContent(), &pod); err != nil {
			return err
		}
		images, err = au.CollectImageInfo(mc.kc, &pod, images, true)
		if err != nil {
			return err
		}
	}

	results, err := mc.collectReports(context.TODO(), images)
	if err != nil {
		return err
	}

	mc.store.Add(collectClusterCVEMetrics(results, mc.generators[offset], mc.generators[offset+1], mc.generators[offset+2]))
	mc.store.Add(collectNamespaceCVEMetrics(images, results, mc.generators[offset+3], mc.generators[offset+4], mc.generators[offset+5]))
	mc.store.Add(collectImageCVEMetrics(results, mc.generators[offset+6], mc.generators[offset+7]))
	mc.store.Add(collectLineageMetrics(images, mc.generators[offset+8]))

	return nil
}

type result struct {
	ref     string
	report  scannerapi.ImageScanReport
	missing bool
}

var severities = []string{
	"CRITICAL",
	"HIGH",
	"MEDIUM",
	"LOW",
	"UNKNOWN",
}

// Based on https://pkg.go.dev/golang.org/x/sync@v0.1.0/errgroup#example-Group-Pipeline
func (mc *Collector) collectReports(ctx context.Context, images map[string]kmapi.ImageInfo) (map[string]result, error) {
	// ctx is canceled when g.Wait() returns. When this version of MD5All returns
	// - even in case of error! - we know that all of the goroutines have finished
	// and the memory they were using can be garbage-collected.
	g, ctx := errgroup.WithContext(ctx)
	requests := make(chan scannerapi.ImageScanRequestSpec)

	g.Go(func() error {
		defer close(requests)
		for ref, info := range images {
			req := scannerapi.ImageScanRequestSpec{
				Image: ref,
			}
			if info.PullCredentials != nil {
				req.Namespace = info.PullCredentials.Namespace
				req.PullSecrets = info.PullCredentials.SecretRefs
				req.ServiceAccountName = info.PullCredentials.ServiceAccountName
			}
			select {
			case requests <- req:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	// Start a fixed number of goroutines to read reports.
	c := make(chan result)
	const maxConcurrency = 5
	for i := 0; i < maxConcurrency; i++ {
		g.Go(func() error {
			for req := range requests {
				var report scannerapi.ImageScanReport
				err := mc.kc.Get(ctx, client.ObjectKey{Name: scannerapi.GetReportName(req.Image)}, &report)
				if client.IgnoreNotFound(err) != nil {
					return err
				} else if apierrors.IsNotFound(err) {
					_ = shared.SendScanRequest(ctx, mc.kc, req.Image, kmapi.PullCredentials{
						Namespace:          req.Namespace,
						SecretRefs:         req.PullSecrets,
						ServiceAccountName: req.ServiceAccountName,
					})
				}
				select {
				case c <- result{
					ref:     req.Image,
					report:  report,
					missing: apierrors.IsNotFound(err),
				}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}
	go func() {
		_ = g.Wait()
		close(c)
	}()

	m := make(map[string]result)
	for r := range c {
		m[r.ref] = r
	}
	// Check whether any of the goroutines failed. Since g is accumulating the
	// errors, we don't need to send them (or check for them) in the individual
	// results sent on the channel.
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return m, nil
}

func collectClusterCVEMetrics(results map[string]result, gen, genO, genC generator.FamilyGenerator) (*metric.Family, *metric.Family, *metric.Family) {
	f := gen.Generate(nil)
	fO := genO.Generate(nil)
	fC := genC.Generate(nil)

	riskOccurrence := map[string]int{} // risk -> occurrence
	riskByCVE := map[string]string{}   // cve -> risk
	vulOccurrence := map[string]int{}  // cve -> occurrence

	for _, r := range results {
		if !r.missing {
			for _, rpt := range r.report.Status.Report.Results {
				for _, tv := range rpt.Vulnerabilities {
					riskOccurrence[tv.Severity]++
					riskByCVE[tv.VulnerabilityID] = tv.Severity
					vulOccurrence[tv.VulnerabilityID]++
				}
			}
		}
	}

	for cve, n := range vulOccurrence {
		m := metric.Metric{
			LabelKeys: []string{
				"cve",
				"namespace",
			},
			LabelValues: []string{
				cve,
				"",
			},
			Value: float64(n),
		}
		f.Metrics = append(f.Metrics, &m)
	}

	riskCount := map[string]int{} // risk -> count
	for _, risk := range riskByCVE {
		riskCount[risk]++
	}

	for _, risk := range severities {
		mO := metric.Metric{
			LabelKeys: []string{
				"severity",
				"namespace",
			},
			LabelValues: []string{
				risk,
				"",
			},
			Value: float64(riskOccurrence[risk]),
		}
		fO.Metrics = append(fO.Metrics, &mO)

		mC := metric.Metric{
			LabelKeys: []string{
				"severity",
				"namespace",
			},
			LabelValues: []string{
				risk,
				"",
			},
			Value: float64(riskCount[risk]),
		}
		fC.Metrics = append(fC.Metrics, &mC)
	}

	return f, fO, fC
}

func collectNamespaceCVEMetrics(images map[string]kmapi.ImageInfo, results map[string]result, gen, genO, genC generator.FamilyGenerator) (*metric.Family, *metric.Family, *metric.Family) {
	f := gen.Generate(nil)
	fO := genO.Generate(nil)
	fC := genC.Generate(nil)

	riskOccurrenceNS := map[string]map[string]int{} // ns -> risk -> occurrence
	riskByCVENS := map[string]map[string]string{}   // ns -> cve -> risk
	vulOccurrenceNS := map[string]map[string]int{}  // ns -> cve -> occurrence

	for ref, ii := range images {
		namespaces := sets.NewString()
		for _, lineage := range ii.Lineages {
			oi := lineage.Chain[len(lineage.Chain)-1]
			namespaces.Insert(oi.Ref.Namespace)
		}
		r, ok := results[ref]
		if ok && !r.missing {
			riskOccurrence := map[string]int{} // risk -> occurrence
			riskByCVE := map[string]string{}   // cve -> risk
			vulOccurrence := map[string]int{}  // cve -> occurrence

			for _, rpt := range r.report.Status.Report.Results {
				for _, tv := range rpt.Vulnerabilities {
					riskOccurrence[tv.Severity]++
					riskByCVE[tv.VulnerabilityID] = tv.Severity
					vulOccurrence[tv.VulnerabilityID]++
				}
			}

			for ns := range namespaces {
				if _, ok := riskOccurrenceNS[ns]; !ok {
					riskOccurrenceNS[ns] = map[string]int{}
				}
				for risk, n := range riskOccurrence {
					riskOccurrenceNS[ns][risk] += n
				}

				if _, ok := riskByCVENS[ns]; !ok {
					riskByCVENS[ns] = map[string]string{}
				}
				for cve, risk := range riskByCVE {
					riskByCVENS[ns][cve] = risk
				}

				if _, ok := vulOccurrenceNS[ns]; !ok {
					vulOccurrenceNS[ns] = map[string]int{}
				}
				for cve, n := range vulOccurrence {
					vulOccurrenceNS[ns][cve] += n
				}
			}
		}
	}

	for ns := range riskOccurrenceNS {
		for cve, n := range vulOccurrenceNS[ns] {
			m := metric.Metric{
				LabelKeys: []string{
					"cve",
					"namespace",
				},
				LabelValues: []string{
					cve,
					ns,
				},
				Value: float64(n),
			}
			f.Metrics = append(f.Metrics, &m)
		}

		riskOccurrence := riskOccurrenceNS[ns] // risk -> occurrence
		riskByCVE := riskByCVENS[ns]           // cve -> risk

		riskCount := map[string]int{} // risk -> count
		for _, risk := range riskByCVE {
			riskCount[risk]++
		}

		for _, risk := range severities {
			mO := metric.Metric{
				LabelKeys: []string{
					"namespace",
					"severity",
				},
				LabelValues: []string{
					ns,
					risk,
				},
				Value: float64(riskOccurrence[risk]),
			}
			fO.Metrics = append(fO.Metrics, &mO)

			mC := metric.Metric{
				LabelKeys: []string{
					"namespace",
					"severity",
				},
				LabelValues: []string{
					ns,
					risk,
				},
				Value: float64(riskCount[risk]),
			}
			fC.Metrics = append(fC.Metrics, &mC)
		}
	}
	return f, fO, fC
}

func collectImageCVEMetrics(results map[string]result, genO, genC generator.FamilyGenerator) (*metric.Family, *metric.Family) {
	fO := genO.Generate(nil)
	fC := genC.Generate(nil)

	for _, r := range results {
		if r.missing {
			continue
		}
		occurrence := map[string]int{}   // risk -> occurrence
		riskByCVE := map[string]string{} // cve -> risk

		for _, rpt := range r.report.Status.Report.Results {
			for _, tv := range rpt.Vulnerabilities {
				occurrence[tv.Severity]++
				riskByCVE[tv.VulnerabilityID] = tv.Severity
			}
		}
		count := map[string]int{}
		for _, risk := range riskByCVE {
			count[risk]++
		}

		for _, risk := range severities {
			mO := metric.Metric{
				LabelKeys: []string{
					"image",
					"severity",
					"namespace",
				},
				LabelValues: []string{
					r.ref,
					risk,
					"",
				},
				Value: float64(occurrence[risk]),
			}
			fO.Metrics = append(fO.Metrics, &mO)

			mC := metric.Metric{
				LabelKeys: []string{
					"image",
					"severity",
					"namespace",
				},
				LabelValues: []string{
					r.ref,
					risk,
					"",
				},
				Value: float64(count[risk]),
			}
			fC.Metrics = append(fC.Metrics, &mC)
		}
	}

	return fO, fC
}

func collectLineageMetrics(images map[string]kmapi.ImageInfo, gen generator.FamilyGenerator) *metric.Family {
	f := gen.Generate(nil)

	for ref, ii := range images {
		for _, lineage := range ii.Lineages {
			for _, oi := range lineage.Chain {
				m := metric.Metric{
					LabelKeys: []string{
						"image",
						"group",
						"kind",
						"namespace",
						"name",
					},
					LabelValues: []string{
						ref,
						oi.Resource.Group,
						oi.Resource.Kind,
						oi.Ref.Namespace,
						oi.Ref.Name,
					},
					Value: 1,
				}
				f.Metrics = append(f.Metrics, &m)
			}
		}
	}
	return f
}
