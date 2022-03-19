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

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	appcatalog "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/pkg/errors"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/zeebo/xxh3"
	atomic_writer "gomodules.xyz/atomic-writer"
	core "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type FileHash struct {
	Data []byte `json:"data,omitempty"`
	Hash uint64 `json:"hash,omitempty"`
}

type ClientBuilder struct {
	tmpDir string
	mgr    manager.Manager

	mu sync.Mutex

	flags    *Config
	w        *atomic_writer.AtomicWriter
	existing map[string]FileHash
	appCfg   *Config

	// last cfg
	cfg *Config
	c   promv1.API
}

func NewBuilder(mgr manager.Manager, flags *Config) (*ClientBuilder, error) {
	dir, err := os.MkdirTemp("/tmp", "prometheus-*")
	if err != nil {
		return nil, err
	}

	w, err := atomic_writer.NewAtomicWriter(strings.TrimSuffix(dir, "/"), "Prometheus ClientBuilder")
	if err != nil {
		return nil, err
	}
	return &ClientBuilder{
		tmpDir:   dir,
		mgr:      mgr,
		flags:    flags,
		w:        w,
		existing: map[string]FileHash{},
	}, nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *ClientBuilder) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	key := req.NamespacedName

	app := &appcatalog.AppBinding{}
	if err := r.mgr.GetClient().Get(ctx, key, app); err != nil {
		klog.Infof("AppBinding %q doesn't exist anymore", req.NamespacedName.String())
		r.unset()
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add or remove finalizer based on deletion timestamp
	if app.ObjectMeta.DeletionTimestamp != nil {
		r.unset()
		return ctrl.Result{}, nil
	}

	cfg, projections, err := r.build(app)
	if err != nil {
		r.unset()
		return ctrl.Result{}, err
	}
	_, err = r.w.Write(projections)
	if err != nil {
		r.unset()
		return ctrl.Result{}, err
	}

	files := make(map[string]FileHash, len(projections))
	for filename, fp := range projections {
		files[filename] = FileHash{
			Data: fp.Data,
			Hash: xxh3.Hash(fp.Data),
		}
	}
	{
		data, _ := json.Marshal(cfg)
		files["prometheus.json"] = FileHash{
			Data: data,
			Hash: xxh3.Hash(data),
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if !hashEquals(r.existing, files) {
		r.appCfg = cfg
		r.existing = files
	}
	return ctrl.Result{}, nil
}

func (r *ClientBuilder) Setup() error {
	if err := r.mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&appcatalog.AppBinding{},
		mona.DefaultPrometheusKey,
		func(rawObj client.Object) []string {
			app := rawObj.(*appcatalog.AppBinding)
			if v, ok := app.Annotations[mona.DefaultPrometheusKey]; ok && v == "true" {
				return []string{"true"}
			}
			return nil
		}); err != nil {
		return err
	}

	authHandler := handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
		var appList appcatalog.AppBindingList
		err := r.mgr.GetClient().List(context.TODO(), &appList, client.MatchingFields{
			mona.DefaultPrometheusKey: "true",
		})
		if err != nil {
			return nil
		}

		var req []reconcile.Request
		for _, app := range appList.Items {
			if app.GetNamespace() == a.GetNamespace() &&
				(app.Spec.Secret != nil && app.Spec.Secret.Name == a.GetName() ||
					app.Spec.TLSSecret != nil && app.Spec.TLSSecret.Name == a.GetName()) {
				req = append(req, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&app)})
			}
		}
		return req
	})

	return ctrl.NewControllerManagedBy(r.mgr).
		For(&appcatalog.AppBinding{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			if v, ok := obj.GetAnnotations()[mona.DefaultPrometheusKey]; ok {
				return v == "true"
			}
			return false
		}))).
		Watches(&source.Kind{Type: &core.Secret{}}, authHandler).
		Complete(r)
}

func (r *ClientBuilder) build(app *appcatalog.AppBinding) (*Config, map[string]atomic_writer.FileProjection, error) {
	var cfg Config

	addr, err := app.URL()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "AppBinding %s/%s contains invalid url", app.Namespace, app.Name)
	}
	cfg.Addr = addr
	cfg.TLSConfig.ServerName = app.Spec.ClientConfig.ServerName
	cfg.TLSConfig.InsecureSkipVerify = app.Spec.ClientConfig.InsecureSkipTLSVerify

	if app.Spec.Secret != nil && app.Spec.Secret.Name != "" {
		var authSecret core.Secret
		key := client.ObjectKey{Namespace: app.Namespace, Name: app.Spec.Secret.Name}
		err = r.mgr.GetClient().Get(context.TODO(), key, &authSecret)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "Secret %s not found", key)
		}
		if u, ok := authSecret.Data[core.BasicAuthUsernameKey]; ok {
			cfg.BasicAuth.Username = string(u)
			if p, ok := authSecret.Data[core.BasicAuthPasswordKey]; ok {
				cfg.BasicAuth.Password = string(p)
			}
		} else {
			if p, ok := authSecret.Data["token"]; ok {
				cfg.BearerToken = string(p)
			}
		}
	}

	projections := map[string]atomic_writer.FileProjection{}
	if len(app.Spec.ClientConfig.CABundle) > 0 {
		projections["ca.crt"] = atomic_writer.FileProjection{
			Data: app.Spec.ClientConfig.CABundle,
			Mode: 0o644,
		}
		cfg.TLSConfig.CAFile = filepath.Join(r.tmpDir, "ca.crt")
	}

	if app.Spec.TLSSecret != nil && app.Spec.TLSSecret.Name != "" {
		var clientSecret core.Secret
		key := client.ObjectKey{Namespace: app.Namespace, Name: app.Spec.TLSSecret.Name}
		err = r.mgr.GetClient().Get(context.TODO(), key, &clientSecret)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "Secret %s not found", key)
		}

		if v, ok := clientSecret.Data[core.TLSCertKey]; ok {
			projections[core.TLSCertKey] = atomic_writer.FileProjection{
				Data: v,
				Mode: 0o644,
			}
			cfg.TLSConfig.CertFile = filepath.Join(r.tmpDir, core.TLSCertKey)
		}
		if v, ok := clientSecret.Data[core.TLSPrivateKeyKey]; ok {
			projections[core.TLSPrivateKeyKey] = atomic_writer.FileProjection{
				Data: v,
				Mode: 0o644,
			}
			cfg.TLSConfig.CertFile = filepath.Join(r.tmpDir, core.TLSPrivateKeyKey)
		}
	}

	return &cfg, projections, nil
}

func (r *ClientBuilder) unset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.appCfg = nil
	r.existing = nil
}

type ChangeStats struct {
	Comment string `json:"comment,omitempty"`
	Diff    string `json:"diff,omitempty"`
}

func hashEquals(existing, updated map[string]FileHash) bool {
	changes := make([]ChangeStats, 0, len(existing)+len(updated))
	for filename, modified := range updated {
		if cur, found := existing[filename]; !found {
			changes = append(changes, ChangeStats{Comment: "[+] " + filename})
		} else if cur.Hash != modified.Hash {
			edits := myers.ComputeEdits(span.URIFromPath("before.txt"), string(cur.Data), string(modified.Data))
			diff := fmt.Sprint(gotextdiff.ToUnified("before.txt", "after.txt", string(cur.Data), edits))
			stats := ChangeStats{Comment: "[~] " + filename, Diff: diff}
			changes = append(changes, stats)
		}
	}
	for filename := range existing {
		if _, found := updated[filename]; !found {
			changes = append(changes, ChangeStats{Comment: "[-] " + filename})
		}
	}

	if len(changes) > 0 {
		klog.Infoln("change detected:")
		for _, change := range changes {
			klog.Infoln(change.Comment)
			if change.Diff != "" {
				klog.Infoln(change.Diff)
			}
		}
	}
	return len(changes) == 0
}

func (r *ClientBuilder) GetPrometheusClient() (promv1.API, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var cfg *Config
	if r.appCfg != nil {
		cfg = r.appCfg
	} else if r.flags != nil && r.flags.Addr != "" {
		cfg = r.flags
	}
	if cfg == nil {
		return nil, nil
	}
	if r.cfg == cfg { // pointer equality
		return r.c, nil
	}

	pc, err := cfg.NewPrometheusClient()
	if err != nil {
		return nil, err
	}
	r.cfg = cfg
	r.c = promv1.NewAPI(pc)
	return r.c, nil
}
