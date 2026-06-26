# Design: `editor.ui.k8s.appscode.com` extended API

| | |
|---|---|
| **Status** | Implemented (read-only scope) |
| **Author** | tamal@appscode.com |
| **Date** | 2026-06-26 |
| **Repos** | `kmodules.xyz/resource-metadata` (types), `kubeops.dev/ui-server` (registry), `go.bytebuilders.dev/b3` (consumer) |

## 1. Summary

The "editor" experience — render a chart's model/manifest/resources from a set
of options, and load those for an existing installation — was implemented in the
AppsCode platform server **b3** as HTTP handlers under
`routers/api/v1/clusters.go` (the `/options` and `/editor` route groups). Those
handlers call `kubepack.dev/lib-app/pkg/editor` directly against a target
cluster reached via a kubeconfig.

This design moves the **read-only** subset of that surface into the cluster as a
native aggregated Kubernetes API group, `editor.ui.k8s.appscode.com/v1alpha1`,
served by `kube-ui-server`. Clients (AceUI / Console, and b3) talk to it through
the Kubernetes API — with RBAC, impersonation, and audit — instead of a bespoke
endpoint.

**Scope: read-only only.** Apply (`PUT /editor/`) and delete (`DELETE /editor/`)
remain entirely in b3 (NATS TaskMgr flow) and are out of scope here.

## 2. Goals / Non-goals

### Goals
- Expose the 6 read-only b3 editor endpoints as aggregated API resources.
- Put the request/response types upstream in `resource-metadata` (alongside the
  other `meta`/`ui` aggregated types), not local to ui-server.
- Reuse the already-vendored `kubepack.dev/lib-app/pkg/editor` logic so behavior
  matches b3.
- **Authorize the resources touched as the extended-API caller**, via
  impersonation.

### Non-goals
- Apply/delete (mutating). Left as-is in b3.
- b3's platform orchestration (NATS task queue, EKS IRSA creds, feature-set
  auto-enable, observability wiring).
- Persisting editor objects — these are non-persisted, create-only RPC resources.

## 3. API (resource-metadata)

New group `editor.ui.k8s.appscode.com`, version `v1alpha1`
(`apis/editor/{doc.go,install,v1alpha1}`). Two create-only, cluster-scoped
action kinds (`+genclient:onlyVerbs=create`, `Request`/`Response` in one object,
like `meta.Render` / `meta.ResourceManifests`). The 6 b3 endpoints collapse to
2 kinds via an `output` discriminator (`model` | `manifest` | `resources`).

### `EditorRender` — render from options (`/options/*`)
```go
type EditorRenderRequest struct {
    ChartRef *releasesapi.ChartSourceFlatRef // optional; else resource-editor chart
    Options  *runtime.RawExtension           // the options/values model
    Output   EditorOutput                    // model | manifest | resources (default resources)
    SkipCRDs bool
}
type EditorRenderResponse struct {
    Model     *runtime.RawExtension       // output=model
    Manifest  string                      // output=manifest
    Resources *releasesapi.ResourceOutput // output=resources
}
```
Backed by `editor.GenerateResourceEditorModel` (model),
`editor.RenderResourceEditorChart` (manifest), and
`editor.RenderChart`/`RenderResourceEditorChart` (resources, with the kubedb-first
sort + `skipCRDs` from b3 `PreviewEditorResources`).

### `EditorTemplate` — load from an existing install (`/editor/{model,manifest,resources}`)
```go
type EditorTemplateRequest struct {
    ChartRef *releasesapi.ChartSourceFlatRef
    Metadata releasesapi.Metadata // resource id + release name/namespace
    Output   EditorOutput
}
type EditorTemplateResponse struct {
    Values    *runtime.RawExtension       // output=model
    Manifest  string                      // output=manifest
    Resources *releasesapi.ResourceOutput // output=resources
}
```
Backed by `editor.LoadEditorModel`/`LoadResourceEditorModel`. Unlike b3's load
handlers, it does **not** call `CreateAppReleaseIfMissing` (that is a write) —
keeping the resource read-only, matching the existing `meta.resourcemanifests`
storage.

> No CRD yaml is generated for these kinds — like `render`/`resourcemanifests`/
> `costreport`, they are served by the aggregated apiserver, not CRD-backed.
> `make gen` produces deepcopy + openapi + clientset only.

## 4. Registry & authorization (ui-server)

Storages under `pkg/registry/editor/{render,template}`, each implementing
`rest.Creater`, registered in `pkg/apiserver/apiserver.go` under a new
`InstallAPIGroup` block (+ `editorinstall.Install(Scheme)`).

**Authorization — act as the caller.** A shared helper
(`pkg/registry/editor/editorutil`) builds, per request, a controller-runtime
client and lib-helm chart registry that **impersonate the API caller**:

```go
user, _ := apirequest.UserFrom(ctx)
impCfg := rest.CopyConfig(cfg)
impCfg.Impersonate = rest.ImpersonationConfig{
    UserName: user.GetName(), UID: user.GetUID(),
    Groups: user.GetGroups(), Extra: user.GetExtra(),
}
kc, _ := client.New(impCfg, client.Options{Scheme, Mapper: mgr.GetRESTMapper()})
reg := repo.NewRegistry(kc, defaultCache)
```

Every read performed while rendering or loading (chart sources, resource
editors, existing objects) is then evaluated by the kube-apiserver against the
caller's own RBAC. This requires `kube-ui-server`'s service account to hold
`impersonate` on `users`/`groups`/`userextras` (chart RBAC).

The shared mapper is reused (identity-independent); only the client's config
varies per caller.

## 5. Consumption (b3)

b3's 6 read-only handlers are rewritten to build the typed `EditorRender` /
`EditorTemplate` object and `Create` it against the target cluster via an
editor-scheme controller-runtime client (lib-helm's client scheme lacks these
types), then return the populated `Response`. Default output formats match b3's
existing behavior (model → JSON, manifest → string, resources → YAML). Apply and
delete handlers are untouched. `clusters.go` routes are unchanged (macaron
resolves handler params by type).

## 6. Cross-repo rollout

1. **resource-metadata** — types + `make gen` (PR #653).
2. **ui-server** — bump resource-metadata, vendor, registry + wiring.
3. **b3** — bump resource-metadata, vendor, rewrite the 6 read handlers.

ui-server and b3 pin resource-metadata to the editor-api commit until #653
merges, after which they move to the released version.

## 7. Notes / future work

- `meta.resourcemanifests` overlaps with `EditorTemplate(output=resources)`; kept
  as-is for back-compat.
- A `format` field was intentionally omitted; resources default to YAML (b3's
  default). b3 can convert for the rare `format=json` case.
- Apply/delete could later be added as authorized (impersonating) mutating
  actions if the platform moves off the NATS TaskMgr flow.
