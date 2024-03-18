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
	"sort"
	"strings"

	reportsapi "kubeops.dev/scanner/apis/reports/v1alpha1"
	scannerapi "kubeops.dev/scanner/apis/scanner/v1alpha1"
	"kubeops.dev/scanner/apis/trivy"
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
	"kmodules.xyz/go-containerregistry/name"
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
	_ rest.SingularNameProvider     = &Storage{}
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

func (r *Storage) GetSingularName() string {
	return strings.ToLower(reportsapi.ResourceKindCVEReport)
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
		images, err = apiutil.CollectImageInfo(r.kc, &pod, images, false)
		if err != nil {
			return nil, err
		}
	}

	// For image, keep ImageInfo if found in any pods or just try as a image name
	if shared.IsImageRequest(oi) {
		ref, err := name.ParseReference(in.Request.Ref.Name)
		if err != nil {
			return nil, err
		}
		if ii, ok := images[ref.Name]; ok {
			images = map[string]kmapi.ImageInfo{
				ref.Name: ii,
			}
		} else {
			images = map[string]kmapi.ImageInfo{
				ref.Name: {
					Image: ref.Name,
				},
			}
		}
	}

	results, err := collectReports(ctx, r.kc, images)
	if err != nil {
		return nil, err
	}

	if shared.IsCVERequest(oi) {
		in.Response, err = GenerateReports(images, results, relevantCVE{cve: oi.Ref.Name})
	} else {
		in.Response, err = GenerateReports(images, results, everything{})
	}

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
				err := kc.Get(ctx, client.ObjectKey{Name: scannerapi.GetReportName(req.Image)}, &report)
				if client.IgnoreNotFound(err) != nil {
					return err
				} else if apierrors.IsNotFound(err) {
					_ = shared.SendScanRequest(ctx, kc, req.Image, kmapi.PullCredentials{
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

type IsRelevant interface {
	Result(r result) bool
	CVE(v trivy.Vulnerability) bool
}

type everything struct{}

var _ IsRelevant = everything{}

func (a everything) Result(r result) bool {
	return true
}

func (a everything) CVE(v trivy.Vulnerability) bool {
	return true
}

type relevantCVE struct {
	cve string
}

var _ IsRelevant = relevantCVE{}

func (a relevantCVE) Result(r result) bool {
	if r.missing {
		return false
	}
	for _, rpt := range r.report.Status.Report.Results {
		for _, tv := range rpt.Vulnerabilities {
			if tv.VulnerabilityID == a.cve {
				return true
			}
		}
	}
	return false
}

func (a relevantCVE) CVE(v trivy.Vulnerability) bool {
	return v.VulnerabilityID == a.cve
}

func GenerateReports(images map[string]kmapi.ImageInfo, results map[string]result, isRelevant IsRelevant) (*reportsapi.CVEReportResponse, error) {
	totalOccurrence := map[string]int{} // risk level -> int

	imginfos := map[string]reportsapi.ImageInfo{}
	vuls := map[string]trivy.VulnerabilityInfo{}

	for ref, r := range results {
		if !isRelevant.Result(r) {
			continue
		}

		riskOccurrence := map[string]int{} // risk -> occurrence
		riskByCVE := map[string]string{}   // cve -> risk

		iis, ok := imginfos[ref]
		if !ok {
			iis = reportsapi.ImageInfo{}
		}
		setImageInfos(&iis, ref, &r.report) // set response.images[].image

		if r.missing {
			iis.ScanStatus.Result = reportsapi.ScanResultNotFound
		} else {
			setImageMetadata(&iis, &r.report.Status.Report.Metadata) // set response.images[].metadata
			setImageScanStatus(&iis, &r.report)                      // set response.images[].scanStatus
			for _, rpt := range r.report.Status.Report.Results {
				for _, tv := range rpt.Vulnerabilities {
					if !isRelevant.CVE(tv) {
						continue
					}

					totalOccurrence[tv.Severity]++
					riskOccurrence[tv.Severity]++
					riskByCVE[tv.VulnerabilityID] = tv.Severity
					populateVulnerabilityInfoMap(vuls, ref, &tv, &rpt)
				}
			}
		}
		setImageStats(&iis, riskOccurrence, riskByCVE) // set response.images[].stats
		imginfos[ref] = iis
	}

	return &reportsapi.CVEReportResponse{
		Images: sortImageInfosByVulnerabilities(imginfos),
		Vulnerabilities: reportsapi.VulnerabilityInfo{
			Stats: getVulnerabilityStats(totalOccurrence, vuls),
			CVEs:  getCVEsFromVulnerabilityInfoMap(vuls),
		},
	}, nil
}

func setImageInfos(ii *reportsapi.ImageInfo, ref string, report *scannerapi.ImageScanReport) {
	ii.Image = reportsapi.ImageReference{
		Name:   ref,
		Tag:    report.Spec.Image.Tag,
		Digest: report.Spec.Image.Digest,
	}
}

func setImageMetadata(ii *reportsapi.ImageInfo, md *trivy.ImageMetadata) {
	var m2 reportsapi.ImageMetadata
	if md.Os.Name != "" || md.Os.Family != "" {
		m2.Os = &md.Os
	}
	cfg := reportsapi.ImageConfig{
		Architecture: md.ImageConfig.Architecture,
		Author:       md.ImageConfig.Author,
		Container:    md.ImageConfig.Container,
		Os:           md.ImageConfig.Os,
	}
	if cfg != (reportsapi.ImageConfig{}) {
		m2.ImageConfig = &cfg
	}
	if m2.Os != nil || m2.ImageConfig != nil {
		ii.Metadata = &m2
		return
	}
	ii.Metadata = nil
}

func setImageScanStatus(ii *reportsapi.ImageInfo, report *scannerapi.ImageScanReport) {
	ii.ScanStatus = reportsapi.ImageScanStatus{
		Result: reportsapi.ScanResultFound,
		ReportRef: &core.LocalObjectReference{
			Name: report.Name,
		},
		TrivyDBVersion: &report.Status.Version.VulnerabilityDB.UpdatedAt,
	}
}

func setImageStats(ii *reportsapi.ImageInfo, riskOccurrence map[string]int, riskByCVE map[string]string) {
	stats := map[string]reportsapi.RiskStats{}
	for risk, n := range riskOccurrence {
		rs := stats[risk]
		rs.Occurrence = n
		stats[risk] = rs
	}
	for _, risk := range riskByCVE {
		rs := stats[risk]
		rs.Count++
		stats[risk] = rs
	}
	ii.Stats = stats
}

func populateVulnerabilityInfoMap(vuls map[string]trivy.VulnerabilityInfo, ref string, tv *trivy.Vulnerability, rpt *trivy.Result) {
	av, ok := vuls[tv.VulnerabilityID]
	if !ok {
		av = trivy.VulnerabilityInfo{
			VulnerabilityID: tv.VulnerabilityID,
			Title:           tv.Title,
			Severity:        tv.Severity,
			PrimaryURL:      tv.PrimaryURL,
			Results:         nil,
			R:               map[string]trivy.ImageResult{},
		}
	}

	ir, ok := av.R[ref]
	if !ok {
		ir = trivy.ImageResult{
			Image:   ref,
			Targets: nil,
		}
	}
	tgt := trivy.Target{
		InstalledVersion: tv.InstalledVersion,
		Target:           rpt.Target,
		Class:            rpt.Class,
		Type:             rpt.Type,
	}
	if tv.Layer.Digest != "" {
		tgt.Layer = &tv.Layer
	}
	ir.Targets = append(ir.Targets, tgt)
	av.R[ref] = ir

	vuls[av.VulnerabilityID] = av
}

func getVulnerabilityStats(totalOccurrence map[string]int, vuls map[string]trivy.VulnerabilityInfo) map[string]reportsapi.RiskStats {
	stats := map[string]reportsapi.RiskStats{}
	for risk, n := range totalOccurrence {
		rs := stats[risk]
		rs.Occurrence = n
		stats[risk] = rs
	}
	// Risk_level -> num_uniq_cves
	for _, vul := range vuls {
		rs := stats[vul.Severity]
		rs.Count++
		stats[vul.Severity] = rs
	}
	return stats
}

func sortImageInfosByVulnerabilities(imginfos map[string]reportsapi.ImageInfo) []reportsapi.ImageInfo {
	images := make([]reportsapi.ImageInfo, 0, len(imginfos))
	for _, ii := range imginfos {
		images = append(images, ii)
	}
	sort.Slice(images, func(i, j int) bool {
		return calculateVulnerabilities(images[i].Stats) < calculateVulnerabilities(images[j].Stats)
	})
	return images
}

func calculateVulnerabilities(stats map[string]reportsapi.RiskStats) int {
	count := 0
	for _, key := range []string{"HIGH", "LOW", "MEDIUM", "CRITICAL", "UNKNOWN"} {
		if val, ok := stats[key]; ok {
			count += val.Count
		}
	}
	return count
}

func getCVEsFromVulnerabilityInfoMap(vuls map[string]trivy.VulnerabilityInfo) []trivy.VulnerabilityInfo {
	cves := make([]trivy.VulnerabilityInfo, 0, len(vuls))
	for _, vul := range vuls {
		processVulnerabilityInfo(&vul)
		cves = append(cves, vul)
	}

	// sort CVEs by severity
	riskRank := map[string]int{
		"CRITICAL": 0,
		"HIGH":     1,
		"MEDIUM":   2,
		"LOW":      3,
		"UNKNOWN":  4,
	}
	sort.Slice(cves, func(i, j int) bool {
		ci, cj := cves[i], cves[j]

		if riskRank[ci.Severity] != riskRank[cj.Severity] {
			return riskRank[ci.Severity] < riskRank[cj.Severity]
		}
		return ci.VulnerabilityID < cj.VulnerabilityID
	})
	return cves
}

func processVulnerabilityInfo(vul *trivy.VulnerabilityInfo) {
	vul.Occurrence = 0
	vul.Results = make([]trivy.ImageResult, 0, len(vul.R))
	for _, r := range vul.R {
		sort.Slice(r.Targets, func(i, j int) bool {
			if r.Targets[i].Type != r.Targets[j].Type {
				return r.Targets[i].Type < r.Targets[j].Type
			}
			return r.Targets[i].Target != r.Targets[j].Target
		})
		vul.Results = append(vul.Results, r)
		vul.Occurrence += len(r.Targets)
	}
	sort.Slice(vul.Results, func(i, j int) bool {
		return vul.Results[i].Image < vul.Results[j].Image
	})

	vul.Occurrence = 0
	for _, result := range vul.Results {
		vul.Occurrence += len(result.Targets)
	}
}
