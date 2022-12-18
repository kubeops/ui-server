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

package reports

import (
	"context"
	"crypto/md5"
	"fmt"
	"sort"

	reportsapi "kubeops.dev/scanner/apis/reports/v1alpha1"
	scannerapi "kubeops.dev/scanner/apis/scanner/v1alpha1"
	"kubeops.dev/ui-server/pkg/graph"
	"kubeops.dev/ui-server/pkg/shared"

	"golang.org/x/sync/errgroup"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/client-go/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Storage struct {
	kc client.Client
}

var (
	_ rest.GroupVersionKindProvider = &Storage{}
	_ rest.Scoper                   = &Storage{}
	_ rest.Storage                  = &Storage{}
	_ rest.Creater                  = &Storage{}
)

func NewStorage(kc client.Client) *Storage {
	return &Storage{
		kc: kc,
	}
}

func (r *Storage) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return reportsapi.SchemeGroupVersion.WithKind(reportsapi.ResourceKindCVEReport)
}

func (r *Storage) NamespaceScoped() bool {
	return false
}

func (r *Storage) New() runtime.Object {
	return &reportsapi.CVEReport{}
}

func (r *Storage) Destroy() {}

func (r *Storage) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in := obj.(*reportsapi.CVEReport)

	var oi *kmapi.ObjectInfo
	if in.Request != nil {
		oi = &in.Request.ObjectInfo
	}
	pods, err := graph.LocatePods(ctx, r.kc, oi)
	if err != nil {
		return nil, err
	}

	images := map[string]kmapi.ImageInfo{}
	for _, p := range pods {
		var pod core.Pod
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(p.UnstructuredContent(), &pod); err != nil {
			return nil, err
		}
		images, err = apiutil.CollectImageInfo(r.kc, &pod, images)
		if err != nil {
			return nil, err
		}
	}

	results, err := collectReports(ctx, r.kc, images)
	if err != nil {
		return nil, err
	}

	in.Response, err = GenerateReports(images, results)
	return in, err
}

type result struct {
	ref     string
	report  scannerapi.ImageScanReport
	missing bool
}

// Based on https://pkg.go.dev/golang.org/x/sync@v0.1.0/errgroup#example-Group-Pipeline
func collectReports(ctx context.Context, kc client.Client, images map[string]kmapi.ImageInfo) (map[string]result, error) {
	// ctx is canceled when g.Wait() returns. When this version of MD5All returns
	// - even in case of error! - we know that all of the goroutines have finished
	// and the memory they were using can be garbage-collected.
	g, ctx := errgroup.WithContext(ctx)
	requests := make(chan scannerapi.ImageScanRequestSpec)

	g.Go(func() error {
		defer close(requests)
		for ref, info := range images {
			req := scannerapi.ImageScanRequestSpec{
				ImageRef: ref,
			}
			if info.PullSecrets != nil {
				req.Namespace = info.PullSecrets.Namespace
				req.PullSecrets = info.PullSecrets.Refs
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
				err := kc.Get(ctx, client.ObjectKey{Name: fmt.Sprintf("%x", md5.Sum([]byte(req.ImageRef)))}, &report)
				if client.IgnoreNotFound(err) != nil {
					return err
				} else if apierrors.IsNotFound(err) {
					_ = shared.SendScanRequest(ctx, kc, req.ImageRef, kmapi.PullSecrets{
						Namespace: req.Namespace,
						Refs:      req.PullSecrets,
					})
				}
				select {
				case c <- result{
					ref:     req.ImageRef,
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

func GenerateReports(images map[string]kmapi.ImageInfo, results map[string]result) (*reportsapi.CVEReportResponse, error) {
	// count := map[string]string{}  // CVE -> risk level
	occurence := map[string]int{} // risk level -> int

	imginfos := map[string]reportsapi.ImageInfo{}
	vuls := map[string]reportsapi.Vulnerability{}

	for ref, r := range results {
		iis, ok := imginfos[ref]
		if !ok {
			iis = reportsapi.ImageInfo{}
		}
		iis.Name.Ref = ref
		iis.Name.Tag = r.report.Spec.Tag
		iis.Name.Digest = r.report.Spec.Digest

		if r.missing {
			iis.ScanStatus.Result = reportsapi.ScanResultNotFound
		} else {
			iis.ScanStatus.ReportRef = &core.LocalObjectReference{
				Name: r.report.Name,
			}
			iis.ScanStatus.LastChecked = r.report.Status.LastChecked
			iis.ScanStatus.TrivyDBVersion = r.report.Status.TrivyDBVersion

			md := r.report.Status.Report.Metadata
			iis.Metadata = reportsapi.ImageMetadata{
				Os: md.Os,
				ImageConfig: reportsapi.ImageConfig{
					Architecture: md.ImageConfig.Architecture,
					Author:       md.ImageConfig.Author,
					Container:    md.ImageConfig.Container,
					Os:           md.ImageConfig.Os,
				},
			}
			iis.Lineages = images[ref].Lineages

			for _, rpt := range r.report.Status.Report.Results {
				for _, tv := range rpt.Vulnerabilities {
					av, ok := vuls[tv.VulnerabilityID]
					if !ok {
						av = reportsapi.Vulnerability{
							VulnerabilityID:  tv.VulnerabilityID,
							PkgName:          tv.PkgName,
							PkgID:            tv.PkgID,
							SeveritySource:   tv.SeveritySource,
							PrimaryURL:       tv.PrimaryURL,
							DataSource:       tv.DataSource,
							Title:            tv.Title,
							Description:      tv.Description,
							Severity:         tv.Severity,
							CweIDs:           tv.CweIDs,
							Cvss:             tv.Cvss,
							References:       tv.References,
							PublishedDate:    tv.PublishedDate,
							LastModifiedDate: tv.LastModifiedDate,
							FixedVersion:     tv.FixedVersion,
							// Results:          tv.Results,
							R: map[string]reportsapi.ImageResult{},
						}
					}
					occurence[tv.VulnerabilityID]++

					ir, ok := av.R[ref]
					if !ok {
						ir = reportsapi.ImageResult{
							ImageRef: ref,
							Targets:  nil,
						}
					}
					ir.Targets = append(ir.Targets, reportsapi.Target{
						Layer:            tv.Layer,
						InstalledVersion: tv.InstalledVersion,
						Target:           rpt.Target,
						Class:            rpt.Class,
						Type:             rpt.Type,
					})
					av.R[ref] = ir

					vuls[av.VulnerabilityID] = av
				}
			}
		}
		imginfos[ref] = iis
	}

	count := map[string]int{} // Risk_level -> num_uniq_cves
	for _, vul := range vuls {
		count[vul.Severity]++
	}

	riskRank := map[string]int{
		"CRITICAL": 0,
		"HIGH":     1,
	}

	resp := reportsapi.CVEReportResponse{
		Images: make([]reportsapi.ImageInfo, 0, len(imginfos)),
		Vulnerabilities: reportsapi.VulnerabilityInfo{
			Count:      count,
			Occurrence: occurence,
			CVEs:       make([]reportsapi.Vulnerability, 0, len(vuls)),
		},
	}
	for _, ii := range imginfos {
		resp.Images = append(resp.Images, ii)
	}
	sort.Slice(resp.Images, func(i, j int) bool {
		return resp.Images[i].Name.Ref < resp.Images[j].Name.Ref
	})

	for _, vul := range vuls {
		vul.Results = make([]reportsapi.ImageResult, 0, len(vul.R))
		for _, r := range vul.R {
			sort.Slice(r.Targets, func(i, j int) bool {
				if r.Targets[i].Type != r.Targets[j].Type {
					return r.Targets[i].Type < r.Targets[j].Type
				}
				return r.Targets[i].Target != r.Targets[j].Target
			})
			vul.Results = append(vul.Results, r)
		}
		sort.Slice(vul.Results, func(i, j int) bool {
			return vul.Results[i].ImageRef < vul.Results[j].ImageRef
		})
		resp.Vulnerabilities.CVEs = append(resp.Vulnerabilities.CVEs, vul)
	}
	sort.Slice(resp.Vulnerabilities.CVEs, func(i, j int) bool {
		ci, cj := resp.Vulnerabilities.CVEs[i], resp.Vulnerabilities.CVEs[j]

		if riskRank[ci.Severity] != riskRank[cj.Severity] {
			return riskRank[ci.Severity] < riskRank[cj.Severity]
		}
		return ci.VulnerabilityID < cj.VulnerabilityID
	})

	return &resp, nil
}
