---
id: gate-tests-metrics-endpoint-auth-test
kind: story
stage: implementing
tags: [testing, security, portal]
parent: null
depends_on: [gate-security-metrics-endpoint-auth]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# `/metrics` auth gating is asserted ABSENT, locking in public-metrics behavior

## Priority
High

## Spec reference
Item: `gate-security-metrics-endpoint-auth`
Acceptance criterion: make `/metrics` opt-in via config or gate behind
`JAMSESH_METRICS_TOKEN` bearer.

## Gap type
test-integrity. `tests/e2e/golden/metrics_endpoint_test.go:42-45`
actively asserts `/metrics` is reachable without any auth headers ("If
/metrics is behind auth, fail loud"). When the security fix lands, that
test must change — but as written it will block the fix.

## Suggested test
```go
// TestMetricsEndpoint_RequiresAuthOrLoopback
//   - without JAMSESH_METRICS_TOKEN: GET /metrics from non-loopback → 404 (not mounted on public listener)
//   - with JAMSESH_METRICS_TOKEN set: missing/bad bearer → 401; correct bearer → 200
//   - direct loopback (UNIX socket or 127.0.0.1) → 200 even with no bearer
```

## Test location (suggested)
`tests/e2e/golden/metrics_endpoint_test.go` (rewrite) and
`internal/portal/router/router_test.go`
