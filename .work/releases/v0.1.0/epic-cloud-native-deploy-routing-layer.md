---
id: epic-cloud-native-deploy-routing-layer
kind: feature
stage: done
tags: [infra]
parent: epic-cloud-native-deploy
depends_on: [epic-cloud-native-deploy-operational-polish]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Cloud-Native Deploy — Routing Layer

## Epic context

- Parent epic: `epic-cloud-native-deploy`
- Position in epic: phase-2 clustered-mode component. Independent of
  lease-fencing and object-storage-sync (can be designed and
  implemented in parallel with lease-fencing once operational-polish
  lands). Becomes operationally meaningful only when paired with
  lease-fencing; the soft-coordinator hint cache integrates with
  lease-acquisition events from lease-fencing.

## Foundation references

- `docs/ARCHITECTURE.md` — "System overview" diagram (router becomes
  the new front-door component in clustered mode).
- `docs/SPEC.md` — "Deployment shape" (router is the new optional
  component this feature introduces).
- `docs/SELF_HOST.md` — clustered deploy recipe section this feature
  authors.
- `internal/portal/router/` and `internal/portal/server/server.go` —
  the chi router and HTTP server entry points the router service
  reverse-proxies to.
- `cmd/jamsesh/` (the local plugin binary) — the `mcp-headers`
  subcommand that needs to emit `Jam-Session-Id` to support MCP
  routing.

## Brief

A small consistent-hashing reverse proxy that routes every request to the
portal pod currently leasing the request's `session_id`. Optional
component — only deployed in clustered mode. Single-instance deploys
skip it entirely.

The router extracts `session_id` from each request (URL path for REST /
git / WebSocket, header for MCP), consistent-hashes it into the pod ring,
and reverse-proxies to the winning backend. It probes pod liveness via
`/readyz` (from `epic-cloud-native-deploy-operational-polish`) to keep
the ring fresh.

## Scope

In:
- A new Go binary in this repo (e.g. `cmd/jamsesh-router/`) — single
  small process, no persistent state.
- `session_id` extraction for every endpoint shape:
  - REST: path segment in `/api/sessions/{id}/...`
  - Git smart-HTTP: path segment in `/git/sessions/{id}.git/...`
  - WebSocket: path or query in upgrade URL
  - MCP: `Jam-Session-Id` header (requires the `jamsesh` binary's
    `mcp-headers` subcommand to start emitting this header — small
    coordinated change documented in this feature)
- Consistent-hash ring with virtual nodes for even load distribution.
- Pod-set discovery via k8s API / DNS SRV records / static config
  (operator's choice; ship at least the k8s + static modes).
- `/readyz` probe for ring membership.
- Graceful pod removal: pod goes Not-Ready → router stops sending it
  new requests → pod drains in-flight → pod exits.
- Routing-decision metrics (which pod handled which session, ring
  rebalances, probe failures).
- Operator docs in `docs/SELF_HOST.md` for the clustered deploy recipe.

Out:
- TLS termination. Cluster ingress (LB, k8s Ingress, Cloud Run) does
  this; the router speaks plain HTTP to portal pods.
- Authentication. Auth is per-request and enforced by portal pods.
- Object-storage interactions. Router is stateless; it doesn't know
  about bare repos at all.
- Lease coordination. The router asks "which pod for this session?";
  the lease layer (`epic-cloud-native-deploy-lease-fencing`) answers.
  In v1 the answer comes from the consistent-hash ring (deterministic);
  later iterations may add a hint table for stickiness across ring
  rebalances.

## Design decisions

Inherited from epic (lease acquisition = pull-with-soft-coordinator;
routing as a separate small Go service). Feature-local:

- **Soft-coordinator hint cache lives in the router.** When a pod
  responds 200 to a session request, the router caches `session_id →
  pod` for a short TTL (default 60s). Subsequent requests for the same
  session within the TTL skip the consistent-hash ring and go straight
  to the cached pod. Cache invalidates on 503 from the cached pod or
  on TTL expiry. No persistence — restart loses the cache, falls back
  to consistent hashing. Avoids a coordinator process while still
  giving "hot" sessions stickiness across ring rebalances.
- **Consistent hashing over leader-election.** The ring is the
  authoritative routing fallback. A pod routed a session it doesn't
  currently lease tries `pg_try_advisory_lock` on demand; success →
  serve, failure → 503 with `Retry-After`, router re-dispatches.
- **MCP coordination via `Jam-Session-Id` header**, not body
  inspection. The local `jamsesh mcp-headers` subcommand (already
  emits the auth header) is extended to also emit `Jam-Session-Id`
  based on the current bound session. Small coordinated change
  documented in this feature.

## Foundation-doc impact

- `docs/ARCHITECTURE.md` — when this feature lands at `stage: done`, add
  a "Horizontal scaling" subsection describing the clustered-mode
  topology (router + pods + Postgres + object storage). Don't
  pre-document.
- `docs/SELF_HOST.md` — clustered deploy recipe section.

## Architectural choice

**Selected: small focused packages inside a new `cmd/jamsesh-router/`
binary; zero new dependencies beyond chi (already in repo).**

Considered:
- *Option A — generic L7 proxy (Envoy / HAProxy / NGINX) with config recipes*:
  smaller code surface, but MCP's session-id-in-header pattern + the
  soft-coordinator hint cache need either Lua/Wasm extensions or a sidecar
  controller. More moving parts for operators.
- *Option B — in-portal routing (each pod proxies to its peer)*:
  defeats the stateless-pod goal; every pod would need cluster awareness.
- **Option C — standalone Go binary** (`cmd/jamsesh-router/`): aligns with
  epic-level decision, full control over header/route extraction, smaller
  operational surface than Envoy + custom config, shares chi/logging/
  metrics primitives with the portal.

Selected: **C**. Reuses existing portal infrastructure
(`internal/portal/logging` for access logs, `internal/portal/metrics` for
the registry, chi for routing semantics). Single-file `main.go` per the
"small focused" guideline; package structure for the algorithm pieces.

## Implementation Units

### Unit 1: Session-ID extraction + consistent-hash ring (algorithms)

**Files**:
- new: `internal/router/extract/extract.go` — pure session-id extraction
- new: `internal/router/extract/extract_test.go`
- new: `internal/router/ring/ring.go` — consistent-hash ring
- new: `internal/router/ring/ring_test.go`

**Story**: `epic-cloud-native-deploy-routing-layer-core`

```go
// internal/router/extract/extract.go
package extract

// SessionID returns the session id encoded in the request, or "" if none
// could be extracted. Supported shapes:
//   - REST: /api/orgs/{orgID}/sessions/{sessionID}/...
//   - Git:  /git/sessions/{sessionID}.git/...
//   - WS:   /ws/sessions/{sessionID}
//   - MCP:  Jam-Session-Id header
//   - /healthz, /readyz, /metrics, /auth/* → "" (routed broadcast or sticky-by-other)
func SessionID(r *http.Request) string
```

```go
// internal/router/ring/ring.go
package ring

// Ring is a consistent-hash ring with virtual-node replication for even
// load distribution. Safe for concurrent use.
type Ring struct { /* ... */ }

// New returns a Ring with vnodes virtual nodes per real pod.
// 100-200 is a reasonable default; smaller = less even, larger = more memory.
func New(vnodes int) *Ring

// SetPods replaces the ring contents with the provided pod set. Atomic
// from a caller's POV (uses copy-on-write internally to avoid mid-read
// inconsistency).
func (r *Ring) SetPods(pods []Pod)

// Get returns the pod responsible for the given key, or zero Pod if the
// ring is empty.
func (r *Ring) Get(key string) Pod

// Pod is a routable backend.
type Pod struct {
    ID      string // stable identifier (e.g. k8s pod name)
    Address string // host:port the proxy targets
}
```

**Implementation Notes**:
- Hash: stdlib `hash/fnv.New64a`. No external dep; collision profile fine
  at session-cardinality scales.
- Vnode allocation: deterministic — `hash(podID + ":" + vnodeIndex)`.
  Lets tests assert specific routings.
- `SetPods` uses copy-on-write (`atomic.Pointer[ringSnapshot]`) so `Get`
  is lock-free and consistent during membership changes.
- Empty ring → `Get` returns zero `Pod`; caller treats as "no backends
  available" (503 with `Retry-After`).
- Special-case routes (`/healthz`, `/readyz`, `/metrics`, OAuth flows,
  `/auth/*`) bypass session routing — `extract.SessionID` returns ""
  and the router uses a round-robin fallback for those.

**Acceptance Criteria**:
- [ ] `extract.SessionID` returns the session id for each of REST / Git /
  WS path shapes and from the `Jam-Session-Id` header for MCP.
- [ ] Returns `""` for `/healthz`, `/readyz`, `/metrics`, `/auth/*`.
- [ ] `ring.New(vnodes)` populates the ring; `SetPods` replaces atomically.
- [ ] Same key routes to the same pod across calls.
- [ ] Adding/removing one pod from a 5-pod ring re-routes at most 1/5
  of keys (consistent-hash invariant; allow ±10% tolerance).
- [ ] `Get` on empty ring returns zero `Pod`.
- [ ] Concurrent `Get` + `SetPods` race-free under `go test -race`.

### Unit 2: Reverse-proxy HTTP service + lifecycle

**Files**:
- new: `cmd/jamsesh-router/main.go` — binary entrypoint
- new: `cmd/jamsesh-router/main_test.go`
- new: `internal/router/proxy/proxy.go` — reverse-proxy handler
- new: `internal/router/proxy/proxy_test.go`
- new: `internal/router/config/config.go` — router-specific config
- new: `internal/router/config/config_test.go`

**Story**: `epic-cloud-native-deploy-routing-layer-service`

```go
// internal/router/proxy/proxy.go
package proxy

// Handler builds an http.Handler that extracts session id, consults the
// ring (or hint cache), and reverse-proxies to the chosen pod.
type Handler struct {
    Extract  func(*http.Request) string                  // typically extract.SessionID
    Ring     *ring.Ring                                  // current pod set
    Hint     *cache.Hint                                 // soft-coordinator cache
    Fallback http.Handler                                // when session id is ""
    Metrics  *metrics.Registry                           // optional; counts decisions
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

```go
// internal/router/config/config.go
type Config struct {
    Bind             string        `yaml:"bind"`              // default ":8080"
    DiscoveryMode    string        `yaml:"discovery_mode"`    // "static" | "kubernetes"
    StaticPods       []string      `yaml:"static_pods"`       // ["host:port", ...]
    KubeNamespace    string        `yaml:"kube_namespace"`    // for k8s mode
    KubeServiceName  string        `yaml:"kube_service_name"` // for k8s mode
    ProbeInterval    time.Duration `yaml:"probe_interval"`    // default 5s
    ProbeTimeout     time.Duration `yaml:"probe_timeout"`     // default 2s
    HintCacheTTL     time.Duration `yaml:"hint_cache_ttl"`    // default 60s
    Vnodes           int           `yaml:"vnodes"`            // default 150
    ShutdownGraceSeconds int       `yaml:"shutdown_grace_s"`  // default 30
}
```

**Implementation Notes**:
- Reverse proxy via `net/http/httputil.ReverseProxy` — handles WebSocket
  upgrade natively when `Director` sets the right URL and `Transport`
  leaves Hop-by-hop headers alone.
- HTTP/1.1 only on the upstream leg (matches portal's `chi.Router`
  shape; WebSockets require 1.1).
- `Handler.ServeHTTP` flow:
  1. `Extract(r)` → sessionID.
  2. If sessionID == "" → `Fallback.ServeHTTP` (round-robin across ring).
  3. Look up hint cache; if hit and pod still in ring, use that pod.
  4. Else `Ring.Get(sessionID)` for the pod.
  5. Build `*httputil.ReverseProxy` targeting the pod's URL.
  6. On 503 from pod: invalidate hint cache for this session, retry once
     against the ring's next preference (consistent-hash next-bucket
     fallback), then propagate the 503 to client.
  7. On 200: write hint cache `sessionID → podID`.
- Env-overlay pattern mirrors `internal/portal/config/config.go` —
  `JAMSESH_ROUTER_<KEY>` naming.
- Validation: `Validate()` rejects empty `Bind`, requires `StaticPods`
  non-empty when `DiscoveryMode==static`, rejects unknown modes.

**Acceptance Criteria**:
- [ ] `cmd/jamsesh-router/main.go` parses config, builds ring + hint
  cache + proxy handler, starts HTTP server, handles SIGTERM with
  configurable grace.
- [ ] REST request `/api/orgs/o/sessions/s/...` reverse-proxies to the
  pod chosen by `ring.Get("s")`.
- [ ] WS upgrade `/ws/sessions/s` proxies through (Upgrade: websocket
  echoed in response).
- [ ] Git request `/git/sessions/s.git/...` proxies through.
- [ ] MCP request with `Jam-Session-Id: s` header proxies through.
- [ ] `/healthz` etc. fall through to round-robin fallback.
- [ ] 503 from pod triggers hint invalidation + single retry, then
  propagates.
- [ ] Graceful shutdown drains in-flight requests within configured
  grace.

### Unit 3: Pod discovery (static + k8s)

**Files**:
- new: `internal/router/discovery/discovery.go` — Discoverer interface
- new: `internal/router/discovery/static.go` — static-config impl
- new: `internal/router/discovery/k8s.go` — k8s API impl
- new: `internal/router/discovery/*_test.go`
- new: `internal/router/readyz/probe.go` — health-poll helper
- new: `internal/router/readyz/probe_test.go`

**Story**: `epic-cloud-native-deploy-routing-layer-discovery`

```go
// internal/router/discovery/discovery.go
package discovery

// Discoverer publishes the current set of healthy pods to a sink at intervals.
type Discoverer interface {
    // Run blocks until ctx is cancelled. On each discovery + probe pass,
    // it calls Publish(pods) with the current healthy subset.
    Run(ctx context.Context, publish func([]ring.Pod)) error
}

// Static returns a Discoverer that polls the configured pod set, checks
// /readyz on each, and publishes the healthy subset.
func Static(addrs []string, probe *readyz.Probe, interval time.Duration) Discoverer

// Kubernetes returns a Discoverer that watches pods backing the given
// service in the given namespace via the k8s client-go informer, probes
// /readyz, and publishes the healthy subset.
func Kubernetes(namespace, serviceName string, probe *readyz.Probe, interval time.Duration) Discoverer
```

```go
// internal/router/readyz/probe.go
package readyz

// Probe checks the readiness endpoint on a list of pods in parallel.
type Probe struct {
    Client  *http.Client // default 2s timeout
    Path    string       // typically "/readyz"
}

// Check returns the subset of addrs whose /readyz returned 200 within
// timeout. Failures are logged but don't propagate.
func (p *Probe) Check(ctx context.Context, addrs []string) []string
```

**Implementation Notes**:
- k8s impl uses `k8s.io/client-go` (new dep — adds ~5MB to binary;
  acceptable for a deployment binary).
- Static impl is the testable default — operators can use it for VM /
  Docker Compose / bare-metal clusters without k8s.
- Probe pass runs on the configured `ProbeInterval` (default 5s). When
  results change, the Discoverer calls `publish(pods)` to update the
  ring.
- All pod URLs are constructed as `http://<addr>` (router-to-pod is
  plain HTTP; TLS terminated at ingress).

**Acceptance Criteria**:
- [ ] Static Discoverer polls configured addrs, probes /readyz,
  publishes healthy subset on each interval.
- [ ] Static + a healthy addr + an unhealthy addr → published set
  contains only the healthy one.
- [ ] Static + addr that becomes healthy → published on next pass.
- [ ] k8s Discoverer integrates with client-go informer (mocked in
  test).
- [ ] Probe.Check parallelizes; N addrs ≤ probe-timeout total.

### Unit 4: Soft-coordinator hint cache

**Files**:
- new: `internal/router/cache/hint.go`
- new: `internal/router/cache/hint_test.go`

**Story**: `epic-cloud-native-deploy-routing-layer-hint-cache`

```go
// internal/router/cache/hint.go
package cache

// Hint is an LRU-bounded in-memory cache mapping session_id → pod ID.
// Entries expire after TTL or on explicit Invalidate.
type Hint struct { /* ... */ }

// New returns a Hint with the given max entries and per-entry TTL.
func New(maxEntries int, ttl time.Duration) *Hint

// Get returns the cached pod ID and true if present and unexpired.
// Returns "", false otherwise.
func (h *Hint) Get(sessionID string) (string, bool)

// Set records or refreshes the sessionID → podID mapping.
func (h *Hint) Set(sessionID, podID string)

// Invalidate drops the entry for sessionID (no-op if absent).
func (h *Hint) Invalidate(sessionID string)
```

**Implementation Notes**:
- LRU: simple wrap of `container/list` + `map[string]*list.Element`
  protected by `sync.Mutex`. Lock-contention concern at very high
  routing rates; mitigate later if measured.
- Entry holds `podID` + `expiry time.Time`. Eviction on `Get` of
  expired entry returns miss.
- `maxEntries` default 10_000 (~hundreds of KB).

**Acceptance Criteria**:
- [ ] Set then Get within TTL → hit with same podID.
- [ ] Get after TTL → miss.
- [ ] Set N entries with maxEntries=N then 1 more → oldest evicted.
- [ ] Invalidate then Get → miss.
- [ ] Concurrent Get/Set under `go test -race` clean.

### Unit 5: `Jam-Session-Id` header in `jamsesh mcp-headers`

**Files**:
- edit: `cmd/jamsesh/mcpheaders/mcpheaders.go`
- edit: `cmd/jamsesh/mcpheaders/mcpheaders_test.go`
- edit: `cmd/jamsesh/state/state.go` — add per-instance session binding
  read helper if not present

**Story**: `epic-cloud-native-deploy-routing-layer-mcp-header`

```go
// Current subcommand outputs:
//   {"Authorization": "Bearer <tok>"}
// New subcommand outputs:
//   {"Authorization": "Bearer <tok>", "Jam-Session-Id": "<sess>"}
// When no session is bound, Jam-Session-Id is omitted (single-instance
// portal doesn't need it; clustered portal falls back to round-robin
// for unrouted MCP calls — they hit any pod and the MCP handler
// extracts session_id from the tool payload as today).
```

**Implementation Notes**:
- Read the per-CC-instance session binding from
  `${CLAUDE_PLUGIN_DATA}/sessions/<cc-session-id>/ref` (per
  `docs/ARCHITECTURE.md` local state layout).
- Tolerate absent binding — emit just the Authorization header.
- This change is universally safe: pods running single-instance ignore
  the header; clustered pods use it for routing.

**Acceptance Criteria**:
- [ ] With token + bound session → header JSON includes both fields.
- [ ] With token, no bound session → header JSON has Authorization only.
- [ ] No token → exits 2 with "no token found" (existing behavior
  preserved).

### Unit 6: Metrics + clustered-deploy docs

**Files**:
- edit: `internal/router/proxy/proxy.go` — emit routing-decision
  metrics via the metrics Registry (if non-nil)
- edit: `cmd/jamsesh-router/main.go` — wire up metrics endpoint at
  `/metrics`
- edit: `docs/SELF_HOST.md` — new "Clustered mode (preview)" section
  explaining how to deploy the router alongside multiple pods
- edit: `docs/ARCHITECTURE.md` — add the "Horizontal scaling" subsection
  per the feature's Foundation-doc impact

**Story**: `epic-cloud-native-deploy-routing-layer-metrics-and-docs`

Routing metrics (added to `internal/portal/metrics/metrics.go` — yes,
reused; the router imports the portal's metrics package):
- `jamsesh_router_decisions_total{result}` — `hit_cache | hit_ring |
  fallback | empty_ring | retry | 503`
- `jamsesh_router_ring_size` — current pod count
- `jamsesh_router_ring_rebalances_total` — counter
- `jamsesh_router_probe_failures_total{addr}` — counter (cardinality
  bounded by pod count)

**Implementation Notes**:
- Reusing `internal/portal/metrics` cross-binary is OK because it's a
  Go package, not a service. Both binaries link it in.
- Docs section: `gcloud run`, `fly`, k8s YAML for the router service.
  Note that the clustered deploy depends on lease-fencing landing too
  (per the epic phasing); link the deploy section to the feature
  ids without external linking — substrate-internal references.

**Acceptance Criteria**:
- [ ] Routing decisions emit metrics with correct labels.
- [ ] `/metrics` on the router exposes Prometheus text format.
- [ ] `docs/SELF_HOST.md` clustered-mode section is concrete and
  references the lease-fencing + object-storage-sync + hydration
  prerequisites.
- [ ] `docs/ARCHITECTURE.md` has the Horizontal scaling subsection.

## Implementation Order

Wave 1 (parallel): Unit 1 (core), Unit 4 (hint cache), Unit 5 (mcp-header)
Wave 2 (parallel): Unit 2 (service), Unit 3 (discovery) — both depend on Unit 1
Wave 3: Unit 6 (metrics + docs) — depends on Unit 2 + Unit 3

## Testing

| Unit | Type | Key surfaces |
|---|---|---|
| 1 extract+ring | unit | all URL shapes; ring add/remove; consistent-hash invariant; concurrent Get/SetPods |
| 2 service | unit + integration | reverse-proxy paths (REST/WS/Git/MCP); 503 retry; graceful shutdown |
| 3 discovery | unit (static) + mocked-k8s (informer fake); probe parallelism |
| 4 hint cache | unit | TTL, LRU eviction, concurrency |
| 5 mcp-header | unit | both fields, missing session, missing token |
| 6 metrics+docs | manual + scrape format test |

## Risks

- **k8s client-go dependency adds binary weight** (~5MB). Worth it for
  the discovery integration; operators not using k8s can build with a
  build tag if size becomes a problem.
- **httputil.ReverseProxy WebSocket support** — well-trodden in Go but
  has historically had subtle bugs around Hijack/Upgrade. Test with the
  actual portal `/ws/sessions/{id}` route during Unit 2 implementation.
- **Hint cache vs ring divergence** — if the ring rebalances while a
  cache entry points to a now-out-of-ring pod, the next request gets
  the old pod's 503 and the router retries via the ring. Acceptable
  cost; document.
- **MCP single-session-per-CC-instance assumption** — the
  `Jam-Session-Id` header reflects the binding at MCP-connection time.
  If the CC instance rebinds mid-connection, the header is stale until
  the next connection. Document this; the existing per-tool-call
  session_id in payloads remains the authoritative source for the
  portal itself.

## Foundation-doc impact

Updated at Unit 6 (when the feature lands):
- `docs/ARCHITECTURE.md` — Horizontal scaling subsection.
- `docs/SELF_HOST.md` — Clustered mode (preview) section.

## Children complete (2026-05-17)

All 6 child stories landed and reviewed across multiple orchestrator waves:

| Story | Verdict | Notes |
|---|---|---|
| core | Approve | extract + ring packages; lock-free atomic-pointer reads |
| hint-cache | Approve | LRU + TTL; sync.Mutex; idiomatic Go |
| mcp-header | Approve | `Jam-Session-Id` in `jamsesh mcp-headers`; incidental /auth/ extract fix |
| service | Approve with comments | `cmd/jamsesh-router/` + proxy + config; agent didn't commit (orchestrator recovered) |
| discovery | Approve | static + k8s; `neverPublished` sentinel; client-go@v0.36.1 |
| metrics-and-docs | review (pending) | 4 router metric handles + SELF_HOST §14 + ARCHITECTURE horizontal-scaling section |

Verification: `go build ./...` clean; `go test ./...` green across all packages.

Feature advanced `implementing → review`. The single review finding (service agent's non-commit recovery) is a process observation, not a blocker.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes** (aggregate concerns; per-line lenses exercised at story level):

- **Capability completeness**: ✓ Standalone `cmd/jamsesh-router/` binary with consistent-hash routing, hint cache, k8s + static discovery, `/readyz` probing, `/metrics` exposition, configurable graceful shutdown. MCP coordination via `Jam-Session-Id` header from the local `jamsesh mcp-headers` subcommand.
- **Foundation-doc alignment**: ✓ SELF_HOST §14 "Clustered mode (preview)" + ARCHITECTURE "Horizontal scaling" subsection added by the metrics-and-docs child. Both accurately mark this as preview pending object-storage-sync + hydration-handoff.
- **Cross-cutting changes**: `Ring.GetNext()` + `Ring.Pods()` added to the core ring package by the service story; `internal/portal/metrics/metrics.go` gained 4 router-specific handles (RouterDecisionsTotal, RouterRingSize, RouterRingRebalancesTotal, RouterProbeFailuresTotal); k8s client-go@v0.36.1 added as a new dependency (~5MB binary growth on the router only).
- **Process observation**: routing-layer-service agent didn't commit; orchestrator recovered cleanly. Worth tracking if it happens again across orchestrator runs.

Routing decisions emit metrics with bounded-cardinality labels (chi route patterns + fixed-enum result values). The hint cache is fully internal (no persistence; restart drops it). All routing/proxying is plain HTTP to portal pods — TLS terminated at cluster ingress per design.

The clustered serving path is partially shipped — the router and lease primitives are in place; object-storage-sync (durability) and hydration-handoff (lease lifecycle) still need to land for true multi-pod production-ready operation. Documented accurately as "preview".
