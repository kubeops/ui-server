# AGENTS.md

This file provides guidance to coding agents (e.g. Claude Code, claude.ai/code) when working with code in this repository.

## Repository purpose

Go module `kubeops.dev/ui-server` — an aggregated Kubernetes API server (`kube-ui-server`) that exposes a grab-bag of read-only and policy-eval APIs **on top of an existing cluster**, primarily for AceUI / AppsCode Console use cases. Headline resources (from `README.md` and the registry layout):

- `identity.k8s.appscode.com` — `WhoAmI` service that returns the requesting user identity.
- `meta.k8s.appscode.com` — `PodView` (live pod resource usage, sourced from Prometheus), object-graph helpers, resource summaries.
- `cost.k8s.appscode.com` — pod/workload cost reporting.
- `offline.k8s.appscode.com` — offline data flow (snapshotted views).
- `policy.k8s.appscode.com` — policy evaluation (gatekeeper + OPA integration).
- `scanner` registry — proxies/serves results from the kubeops scanner.

The produced binary is `kube-ui-server`. Long-running aggregated apiserver.

## Architecture

- `cmd/kube-ui-server/` — main binary entry point.
- `cmd/objectfinder-tester/` — auxiliary debugging tool for the object-graph engine.
- `pkg/cmds/` — Cobra command tree (root, run).
- `pkg/apiserver/` — aggregated apiserver bootstrap (scheme, recommended options).
- `pkg/registry/` — REST storage per API group:
  - `identity/`, `cost/`, `offline/`, `policy/`, `core/`, `meta/`, `scanner/`.
  - `registry.go` — shared registration glue.
- `pkg/controllers/` — controllers that back the apiserver:
  - `clusterclaim/`, `clustermetadata/` — cluster identity discovery.
  - `feature/` — feature enablement state.
  - `projectquota/` — project-quota tracking.
  - `scanner/` — vulnerability scan cache.
- `pkg/graph/` — Kubernetes resource graph engine. Returns parent/child/related objects for arbitrary resources (used by `meta` registry).
- `pkg/menu/` — UI menu builder. The cluster-aware navigation surface for the UI.
- `pkg/metricshandler/`, `pkg/metricsstore/` — Prometheus self-metrics for the server.
- `pkg/shared/` — shared helpers.
- `apis/`:
  - `cost/v1alpha1/`, `offline/v1alpha1/`, `policy/v1alpha1/`, `identity/v1alpha1/` — types for the served API groups.
- `crds/` — generated CRD YAMLs.
- `artifacts/` — sample manifests / fixtures.
- `Dockerfile.in` (PROD, distroless), `Dockerfile.dbg` (debian), `Dockerfile.ubi` (Red Hat certified) — three image variants.
- `hack/`, `Makefile` — AppsCode build harness.
- `vendor/` — checked-in deps.

CRD API groups all use the `k8s.appscode.com` domain (`cost.k8s.appscode.com`, etc.).

## Common commands

All Make targets run inside `ghcr.io/appscode/golang-dev` — Docker must be running.

- `make ci` — CI pipeline.
- `make build` / `make all-build` — build host or all-platform binaries.
- `make gen` — regenerate clientset + manifests + openapi. Run after changes to `apis/**/*_types.go`.
- `make manifests` — regenerate CRDs only.
- `make clientset` — regenerate client code.
- `make openapi` — regenerate OpenAPI definitions.
- `make fmt`, `make lint`, `make unit-tests` / `make test` — standard.
- `make verify` — `verify-gen verify-modules`; `go mod tidy && go mod vendor` must leave the tree clean.
- `make container` — build PROD, DBG, and UBI images.
- `make push` — push all three; `make docker-manifest` writes multi-arch manifests; `make release` is the full publish flow.
- `make push-to-kind` / `make deploy-to-kind` — load into Kind and Helm-install.
- `make install` / `make uninstall` / `make purge` — Helm install lifecycle.
- `make add-license` / `make check-license` — manage license headers.

Run a single Go test (requires a local Go toolchain):

```
go test ./pkg/registry/meta/... -run TestName -v
```

## Conventions

- Module path is `kubeops.dev/ui-server` (vanity URL). Imports must use that. Binary name is `kube-ui-server`.
- License: see `LICENSE`. Sign off commits (`git commit -s`); contributions follow the DCO.
- Vendor directory is checked in — `go mod tidy && go mod vendor` must leave the tree clean (enforced by `verify-modules`).
- This is an **aggregated apiserver**. New API surfaces go in `pkg/registry/<group>/` plus matching types under `apis/<group>/v1alpha1/`. Do not introduce parallel HTTP handlers outside the apiserver framework.
- The Kubernetes resource-graph engine (`pkg/graph/`) is consumed by the UI to walk object relationships — preserve its query surface when refactoring.
- Do not hand-edit `zz_generated.*.go` or `crds/*.yaml` — change `apis/<group>/v1alpha1/*_types.go` and re-run `make gen`.
- Three Dockerfiles, one binary — keep `Dockerfile.in`, `Dockerfile.dbg`, and `Dockerfile.ubi` in sync.
