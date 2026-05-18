---
id: epic-e2e-cnd-coverage-operational-polish-metrics
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-operational-polish
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# `/metrics` coverage — golden

## Scope

One golden test asserting the portal's `/metrics` endpoint emits valid
Prometheus exposition format, parses cleanly via `expfmt.TextParser`,
and contains at least one well-known metric family.

## Files

- `tests/e2e/golden/metrics_endpoint_test.go`
- `tests/e2e/go.mod` (add `github.com/prometheus/common` as a dependency
  if not already pulled by another fixture)

## Acceptance criteria

- [ ] `/metrics` returns 200 with `Content-Type` containing `text/plain`
      (Prometheus exposition format)
- [ ] `expfmt.TextParser.TextToMetricFamilies` succeeds (no parse error)
- [ ] Returned map is non-empty
- [ ] Spot-check a well-known metric family is present (preferred:
      `go_goroutines` — universal default from prom client lib). If the
      portal disables Go-runtime collectors, substitute a portal-specific
      counter that's always exported; assertion must NOT silently pass
      on an empty map
- [ ] If `/metrics` requires auth, test fails loudly with the 401 —
      implementer adds the auth header in a 1-line fix and re-runs

## Test integrity (from parent epic)

- Asserting only on status code is tautological — a 200 from a
  misconfigured exporter is still wrong if the body isn't valid Prom
  format. The `expfmt` parse + family-presence assertion is the
  non-tautological core.
- If `/metrics` is open in production but the e2e portal image
  inadvertently disables it, that's a real bug — park it, don't change
  the test to "if 404, skip".

## References

- Parent feature body, Unit 2 — full scaffold
- `cmd/portal/main.go:44` — imports `jamsesh/internal/portal/metrics`
- `cmd/jamsesh-router/main.go:145` — router's `/metrics` mount as
  reference for unauth convention
- `github.com/prometheus/common/expfmt` — Prom textparser
