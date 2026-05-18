---
id: epic-e2e-cnd-coverage-operational-polish-metrics
kind: story
stage: done
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-operational-polish
depends_on: []
release_binding: v0.1.0
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

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: All acceptance criteria met. The test correctly asserts on
`text/plain` Content-Type, parses with `expfmt.TextParser` (the
non-tautological core per story design), verifies non-empty families, and
spot-checks `go_goroutines` with a documented fallback to known prefixes that
still fails if nothing recognizable is present. The 401 fail-loud path
correctly names the `WWW-Authenticate` header. `familyNames` helper produces
deterministic failure messages. go.mod additions are correctly indirect. No
tautological assertions; no status-code-only checks. Metrics mount confirmed
unauthenticated in router.go:98 matching the story's note. No foundation-doc
drift; no security or breaking-change concerns.

## Implementation notes

- **Endpoint location confirmed**: `/metrics` is mounted unauthenticated in
  `internal/portal/router/router.go:98` via `r.Mount("/metrics", d.MetricsHandler)`.
  The router comment explicitly says "unauthenticated; operators secure via
  network policy" — no auth required.
- **Metrics always present**: `internal/portal/metrics/metrics.go` calls
  `collectors.NewGoCollector()` and `collectors.NewProcessCollector()` in
  `New()`, so `go_goroutines` and `process_*` families are always registered.
  The spot-check targets `go_goroutines` as the primary well-known family.
- **Dependencies added to `tests/e2e/go.mod`**: `github.com/prometheus/common
  v0.66.1` (direct), `github.com/prometheus/client_model v0.6.2` (direct),
  plus transitive pulls of `google.golang.org/protobuf v1.36.8` and
  `github.com/munnerz/goautoneg`. Versions match the main module's go.mod to
  avoid conflicting transitive resolution.
- **`familyNames` helper**: returns sorted family names for deterministic
  failure messages — avoids non-deterministic map iteration in t.Fatalf output.
- **Auth escape hatch**: if `/metrics` ever returns 401, the test fails loudly
  with the `WWW-Authenticate` header value named explicitly. No silent skip.
- **`go build ./golden/... && go vet ./golden/...`** both pass clean.
