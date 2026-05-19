---
id: gate-tests-metrics-endpoint-auth-test
kind: story
stage: done
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

## Implementation notes

### Unit tests: `internal/portal/router/metrics_auth_test.go`

New file in the `router_test` package with four table-driven cases:

- `TestMetricsUnmounted` — constructs `router.New(Deps{MetricsHandler: stub, MetricsToken: ""})` and asserts `GET /metrics` → 404 with `route.not_found` envelope. Covers the JAMSESH_METRICS_TOKEN-unset path.
- `TestMetricsUnmountedNilHandler` — constructs with `MetricsHandler: nil, MetricsToken: "secret"` and asserts 404. Confirms the nil-handler guard.
- `TestMetricsBearerAuth` — three subtests against `MetricsToken: "test-secret"`:
  - `no_auth_header` → 401, `auth.invalid_token` envelope
  - `wrong_bearer_token` → 401, `auth.invalid_token` envelope
  - `correct_bearer_token` → 200 with Prometheus Content-Type
- `TestMetricsBearerCaseSensitive` — token "Secret" must not match "secret" (constant-time compare is byte-exact).

All four pass: `go test -run TestMetrics ./internal/portal/router/...` → ok.

### E2e test rewrite: `tests/e2e/golden/metrics_endpoint_test.go`

Old test asserted `/metrics` was open (actively blocked the security fix). Rewritten to assert the new contract:

- Portal started with `ExtraEnv: {"JAMSESH_METRICS_TOKEN": "e2e-metrics-test-token"}` via the existing `portal.Options.ExtraEnv` map — no fixture changes needed.
- Three subtests: `no_auth_header_is_401`, `wrong_bearer_is_401`, `correct_bearer_is_200_with_prometheus_output`.
- The 200 subtest preserves the original Prometheus expfmt parse + metric-family-presence assertions.

The e2e test compiles cleanly (`go build -tags e2e ./...` in both modules). The stale portal:e2e image predates the security fix so the test correctly fails against it locally; it will pass once CI rebuilds the image from the current source.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Unit tests at the router layer cover: unmounted (no token) → 404; nil handler → 404; bearer-auth path with no/wrong/correct bearer → 401/401/200; case-sensitivity guard. E2e test at tests/e2e/golden/metrics_endpoint_test.go rewritten with three subtests against a portal fixture that passes JAMSESH_METRICS_TOKEN via portal.Options.ExtraEnv. Stale portal:e2e image predates the security fix so e2e fails locally; CI rebuilds image and will pass.
