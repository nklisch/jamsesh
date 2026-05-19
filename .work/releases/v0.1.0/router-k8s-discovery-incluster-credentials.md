---
id: router-k8s-discovery-incluster-credentials
kind: story
stage: done
tags: [infra, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Router k8s discoverer should auto-load in-cluster CA + service-account token

## Origin

Found during review of `epic-e2e-cnd-coverage-routing-layer-k8s-discovery`.
That story re-introduced `internal/router/discovery/k8s.go` (a plain-HTTP
Endpoints watcher with no client-go dep) and wired it into config. The test
exercises it against an unauthenticated `httptest.Server`, so the
production-deployment credential paths were never required.

For an actual in-cluster deployment, two things are missing:

1. **In-cluster CA cert**. `K8sConfig.HTTPClient` is a vanilla `http.Client`
   by default. Requests to `https://kubernetes.default.svc` will fail with
   x509-verification errors because the in-cluster CA at
   `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt` is not in the
   system trust store.

2. **Service-account bearer token**. `K8sConfig.BearerToken` is empty by
   default. The k8s API server requires authentication for Endpoints reads
   even with RBAC granting the SA the right verbs. The token lives at
   `/var/run/secrets/kubernetes.io/serviceaccount/token` and rotates.

## Fix direction

Add a constructor `K8sInCluster() (K8sConfig, error)` in
`internal/router/discovery/k8s.go` that:

- Reads the SA token file once at startup; populates `BearerToken`.
  Optionally re-reads on each request (the token rotates every few hours;
  read-on-request keeps the watcher alive across rotation).
- Reads the CA cert and constructs an `*http.Client` with a `tls.Config`
  whose `RootCAs` includes it.
- Sets `APIServerURL` to `https://kubernetes.default.svc` if env var
  `KUBERNETES_SERVICE_HOST` is set, otherwise leaves the caller to provide
  it.

Wire this in `cmd/jamsesh-router/main.go` when `DiscoveryMode = kubernetes`
and no explicit `APIServerURL` / `BearerToken` overrides are present.

Add a test that mocks the SA mount via a temp directory + a `K8sInCluster`
variant that takes the mount path as an arg (matching how other in-cluster
patterns in the codebase are testable — check `internal/portal/config/` for
existing precedents).

## Acceptance

- `K8sInCluster` reads CA + SA token from the standard mount path.
- Default `K8sConfig.HTTPClient` (when constructed via `K8sInCluster`) trusts
  the in-cluster CA.
- All requests carry `Authorization: Bearer <token>`.
- Token rotation does not require process restart (read-on-request, or
  goroutine refresh — pick the simpler pattern).
- A unit test mocks the mount path and verifies CA + token are wired
  correctly.
- `docs/SELF_HOST.md` k8s section documents the required RBAC verbs:
  `get`, `list`, `watch` on `endpoints` in the portal namespace.

## Out of scope

- Switching to client-go (the plain-HTTP watcher is intentional — keep it).
- Watching additional resources beyond Endpoints (e.g. EndpointSlice in
  newer clusters — separate story if needed).

## Implementation notes

### Files changed

- `internal/router/discovery/k8s_incluster.go` — new file; `K8sInCluster` constructor and `tokenInjectingRoundTripper` type.
- `internal/router/discovery/k8s_incluster_test.go` — new file; 5 unit tests.
- `internal/router/config/config.go` — relaxed `KubeAPIServerURL` validation to allow empty when `KUBERNETES_SERVICE_HOST` is set.
- `cmd/jamsesh-router/main.go` — wiring: uses `K8sInCluster` when `KubeAPIServerURL` and `KubeBearerToken` are both unset; falls back to explicit-config path otherwise.
- `docs/SELF_HOST.md` — updated k8s RBAC snippet: `pods` → `endpoints` with correct `roleRef` (the prior YAML had a malformed `roleBinding` key).

### Mount-path override mechanism

`K8sInClusterOptions.MountPath` overrides the default `/var/run/secrets/kubernetes.io/serviceaccount`. Tests pass `t.TempDir()` containing synthetic `ca.crt` and `token` files. No mocking of `os.ReadFile` is needed — the tests actually write and read the files, which means rotation behavior (overwriting the token file between requests) is exercised as a true file-system round-trip.

### Token-rotation approach

Read-on-request via `tokenInjectingRoundTripper`. On each `RoundTrip` call the wrapper reads the token file, trims whitespace, and injects `Authorization: Bearer <token>` on a cloned request before delegating to the base transport. This is simpler than a goroutine-based refresh and requires no synchronization — the OS guarantees atomic rename-then-read for file rotation (kubelet rotates tokens via rename). The overhead is one `os.ReadFile` per Endpoints list/watch call, negligible compared to the network round-trip. Rationale is documented inline in `k8s_incluster.go`.

### RBAC docs location

`docs/SELF_HOST.md` §14 "Kubernetes deployment" — the RBAC manifest block was updated from `resources: ["pods"]` to `resources: ["endpoints"]` with verbs `get`, `list`, `watch`. The malformed `roleBinding:` key was corrected to `roleRef:`. The `serviceAccountName` comment in the router Deployment was also updated.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: Read-on-request for token rotation adds an `os.ReadFile` per
Endpoints request. For long-lived watch streams this is rare and negligible;
for the initial list it's a few ms. Worth keeping in mind if router-startup
latency ever becomes a focus, but a goroutine-refresh design wouldn't really
help (the kubelet rotates infrequently, ~hourly).

**Notes**: Solid implementation. Read-on-request rotation is the right design
trade-off — simpler than goroutine refresh, no synchronization needed, OS
rename-then-read atomicity guarantees a clean swap. The `MountPath` override
in `K8sInClusterOptions` made the test truly exercise file rotation rather
than mocking `os.ReadFile`. The bonus fix to `docs/SELF_HOST.md` §14 (RBAC
verbs `pods → endpoints` and `roleBinding: → roleRef:`) corrects YAML that
would not have applied as-shipped — adjacent-scope but unambiguously correct
and worth landing in the same commit.
