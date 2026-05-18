---
id: epic-cloud-native-deploy-operational-polish-metrics
kind: story
stage: done
tags: [infra, portal]
parent: epic-cloud-native-deploy-operational-polish
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Operational Polish — `/metrics` Prometheus endpoint

## Scope

Add `github.com/prometheus/client_golang` as a dependency and expose
a `/metrics` endpoint that surfaces standard Go runtime metrics plus
portal-specific counters (HTTP requests, git pushes, auto-merger
outcomes, event-log throughput).

Implements **Unit 2** of `epic-cloud-native-deploy-operational-polish`.
See parent feature body for full design rationale, including the
cardinality-safety rule about chi route patterns.

## Files

New:
- `internal/portal/metrics/metrics.go` — Registry + typed metric handles
- `internal/portal/metrics/metrics_test.go`

Edit:
- `internal/portal/router/router.go` — mount `/metrics` (unauth)
- `internal/portal/logging/access.go` — emit
  `http_requests_total` + `http_request_duration_seconds` per request
- `cmd/portal/main.go` — construct Registry; thread to router and to
  postreceive emitter
- `go.mod` / `go.sum` — add `github.com/prometheus/client_golang`

## Interface

```go
// internal/portal/metrics/metrics.go
package metrics

type Registry struct {
    HTTPRequestsTotal    *prometheus.CounterVec   // method, route, status
    HTTPRequestDuration  *prometheus.HistogramVec // method, route
    GitPushesTotal       *prometheus.CounterVec   // result
    AutoMergerOutcomes   *prometheus.CounterVec   // outcome
    EventLogEmitTotal    prometheus.Counter
    // ... unexported reg + collectors
}

func New() *Registry
func (r *Registry) Handler() http.Handler
```

Initial portal-specific metrics:
- `http_requests_total{method, route, status}` (counter)
- `http_request_duration_seconds{method, route}` (histogram, default
  buckets 5ms–10s)
- `jamsesh_git_pushes_total{result}` where result ∈ {ok, rejected}
- `jamsesh_automerger_outcomes_total{outcome}` where outcome ∈
  {succeeded, conflict, backpressure}
- `jamsesh_event_log_emit_total` (counter)
- Standard Go runtime + process collectors (auto)

## Acceptance criteria

- [ ] `GET /metrics` returns Prometheus text exposition format.
- [ ] Standard go-runtime metrics present
  (`go_goroutines`, `go_memstats_*`, `process_cpu_seconds_total`).
- [ ] `http_requests_total` increments on every request with method,
  route, status labels.
- [ ] Route labels are chi route patterns
  (`/api/orgs/{orgID}/sessions/{sessionID}`), NOT raw URLs — test
  this explicitly to catch cardinality regressions.
- [ ] `jamsesh_git_pushes_total` increments from the post-receive
  flow.
- [ ] `jamsesh_automerger_outcomes_total` increments from the
  auto-merger applier.
- [ ] `/metrics` endpoint requires no authentication.
- [ ] Unit tests cover: handler returns valid exposition format,
  counter increment after fake request, route-pattern label assertion.

## Implementation notes

### What landed

**New package** `internal/portal/metrics/` (`metrics.go` + `metrics_test.go`):
- `Registry` struct with typed handles: `HTTPRequestsTotal`, `HTTPRequestDuration`,
  `GitPushesTotal`, `AutoMergerOutcomes`, `EventLogEmitTotal`.
- Isolated `*prometheus.Registry` (not the global default) with `GoCollector` and
  `ProcessCollector` for standard runtime/process metrics.
- `Handler()` returns a `promhttp.HandlerFor(r.reg, ...)` — unauthenticated by design.
- Histogram buckets: `ExponentialBuckets(0.005, 2, 12)` → 5ms to ~10s.

**Router** (`internal/portal/router/router.go`):
- Added `MetricsHandler http.Handler` and `MetricsRegistry *metrics.Registry` to `Deps`.
- Mounts `/metrics` after `/readyz` (unauthenticated, nil-guarded).
- Changed `r.Use(logging.Access)` → `r.Use(logging.Access(d.MetricsRegistry))`.

**Logging middleware** (`internal/portal/logging/logging.go`):
- `Access` is now a constructor `func Access(reg *metrics.Registry) func(next http.Handler) http.Handler`.
- Extracts chi route pattern via `chi.RouteContext(r.Context()).RoutePattern()` AFTER
  `next.ServeHTTP` returns (pattern fully resolved post-routing). Falls back to
  sentinel `"unknown"` when RouteContext is nil or pattern is empty.
- Increments `HTTPRequestsTotal` and observes `HTTPRequestDuration` when `reg != nil`.
- Existing tests updated: `logging.Access(inner)` → `logging.Access(nil)(inner)`.

**Event log** (`internal/portal/events/log.go`):
- Added optional `metrics *metrics.Registry` field + `WithMetrics(reg) *Log` chaining method.
- `Emit` increments `EventLogEmitTotal` by 1 after successful DB commit.
- `EmitBatch` increments `EventLogEmitTotal` by `len(drafts)` after successful DB commit.

**Git HTTP handler** (`internal/portal/githttp/handler.go`, `receive_pack.go`):
- Added `Metrics *metrics.Registry` field to `Handler`.
- Increments `GitPushesTotal{result="rejected"}` on pre-receive rejection.
- Increments `GitPushesTotal{result="ok"}` after subprocess exits 0 and post-receive events emit.

**Auto-merger** (`internal/portal/automerger/outcomes.go`, `worker.go`):
- Added `Metrics *metrics.Registry` to both `Applier` and `Worker`.
- `applySuccess` increments `AutoMergerOutcomes{outcome="succeeded"}`.
- `applyConflict` increments `AutoMergerOutcomes{outcome="conflict"}`.
- `emitBackpressure` (Worker) increments `AutoMergerOutcomes{outcome="backpressure"}`.

**Main wiring** (`cmd/portal/main.go`):
- Constructs `metricsReg := metrics.New()` after logging setup.
- Threads to: `eventLog.WithMetrics(metricsReg)`, `gitHandler.Metrics`, `mergerApplier.Metrics`,
  `mergerWorker.Metrics`, `router.Deps.MetricsHandler`, `router.Deps.MetricsRegistry`.

### Test results

9/9 metrics tests pass (`TestHandlerReturnsPrometheusFormat`, `TestStandardGoRuntimeMetricsPresent`,
`TestHTTPRequestsCounterIncrements`, `TestHTTPRequestDurationHistogramEmits`,
`TestRoutePatternLabelUsesChiPattern`, `TestGitPushesTotalIncrements`,
`TestAutoMergerOutcomesIncrements`, `TestEventLogEmitTotalIncrements`,
`TestMetricsHandlerRequiresNoAuthentication`).

Router and logging tests unaffected. Pre-existing `db.Open` signature failures
in many test packages are a wave-level issue from a parallel story; they existed
before this story and are not caused by these changes.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `events.Log` field `metrics *metrics.Registry` shadows the imported package name `metrics`. Compiles cleanly because the type reference is qualified, but slightly confusing. A `registry` or `metricsReg` field name would read better.
- Histogram buckets diverge from the design hint (`ExponentialBuckets(0.005, 2, 12)` ≈ 5ms→10s rather than `prometheus.DefBuckets`). Well-justified, well-documented; not a finding.

**Notes**: Implementation is cleanly designed. Isolated `*prometheus.Registry` (not the global default — important for test hygiene). `New()` registers GoCollector + ProcessCollector so standard runtime metrics show up at `/metrics` without further wiring. All instrumentation sites guard `if X.Metrics != nil` (`Applier`, `Worker`, `Handler`, `Log`), preserving nil-safety for tests and partial deployments.

Cardinality safety is the critical design risk and it's handled correctly: route labels come from `chi.RouteContext(r.Context()).RoutePattern()` extracted AFTER `next.ServeHTTP` returns (so the pattern is resolved), with `"unknown"` sentinel for unmatched routes. The `TestRoutePatternLabelUsesChiPattern` test (per implementation notes) exercises this explicitly.

`logging.Access` signature change (`func(http.Handler) http.Handler` → `func(*metrics.Registry) func(http.Handler) http.Handler`) is package-internal API; all call sites updated (router.go, logging_test.go).

`/metrics` and the new `github.com/prometheus/client_golang` dependency aren't yet in SELF_HOST.md or SPEC.md — belongs to the sibling docs story.
