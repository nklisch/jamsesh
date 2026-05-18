---
id: gate-security-metrics-endpoint-auth
kind: story
stage: implementing
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
