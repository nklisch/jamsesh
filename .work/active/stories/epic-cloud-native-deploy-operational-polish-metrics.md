---
id: epic-cloud-native-deploy-operational-polish-metrics
kind: story
stage: implementing
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
