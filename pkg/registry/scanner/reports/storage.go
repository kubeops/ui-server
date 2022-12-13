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
	"encoding/json"
	"fmt"

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

	// TODO: combine report
	data, err := json.Marshal(results)
	if err != nil {
		return nil, err
	}
	fmt.Println(string(data))

	return in, nil
}

type result struct {
	ref     string
	report  scannerapi.ImageScanRequest
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
				var report scannerapi.ImageScanRequest
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
