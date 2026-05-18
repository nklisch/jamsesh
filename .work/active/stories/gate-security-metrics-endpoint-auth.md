---
id: gate-security-metrics-endpoint-auth
kind: story
stage: review
tags: [security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# `/metrics` endpoint exposes Prometheus data without authentication

## Severity
Medium

## Domain
API Security

## Location
`internal/portal/router/router.go:96-99`

## Evidence
```go
if d.MetricsHandler != nil {
    r.Mount("/metrics", d.MetricsHandler)
}
```

Comment says "operators secure via network policy", but for self-hosted
single-instance deployments (the default `deploy_mode=single`) there is
no upstream network policy and the binary listens publicly. Metrics
include request counts per route, lease acquire totals, git push
counters with `result` labels, and pod identifiers — useful for an
attacker mapping the deployment, sizing pushes, and timing brute-force
attempts.

## Remediation direction
Make `/metrics` opt-in via config (e.g. `metrics.bind` separate listener
on loopback or a UNIX socket), or gate behind a static bearer-token
check seeded from `JAMSESH_METRICS_TOKEN` env. Keep the public mount
only when the operator opts in.

## Implementation notes

### Bearer-token middleware

`metricsTokenMiddleware` in `internal/portal/router/router.go` wraps the
Prometheus handler. It extracts the bearer token from the `Authorization`
header (`strings.TrimPrefix(..., "Bearer ")`) and compares it against the
configured token using `subtle.ConstantTimeCompare` to prevent timing
side-channels. Any mismatch or missing header returns a standard
`httperr` 401 envelope (`auth.invalid_token`).

### Unset-means-404 (default-deny)

The `/metrics` route is only registered when **both** `d.MetricsHandler != nil`
and `len(d.MetricsToken) > 0`. When `JAMSESH_METRICS_TOKEN` is unset (the
default), `cfg.MetricsToken` is the empty string, so the route is never
mounted and the path falls through to chi's 404 handler. Operators who
want Prometheus scraping must explicitly configure the token.

### Config wiring

- `Config.MetricsToken string` added to `internal/portal/config/config.go`
  with yaml tag `metrics_token` and env binding `JAMSESH_METRICS_TOKEN` in
  `applyMetricsEnv`.
- `router.Deps.MetricsToken string` added in `internal/portal/router/router.go`.
- `cmd/portal/main.go` passes `cfg.MetricsToken` into `router.Deps`.

### E2E test follow-on

`tests/e2e/golden/metrics_endpoint_test.go` currently asserts the old
unauthenticated contract (expects 200 without a token). That test will fail
against the new behavior. The companion story
`gate-tests-metrics-endpoint-auth-test` (which depends_on this story) is
responsible for updating the e2e test to match the new authenticated contract.
