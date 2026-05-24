---
id: feature-e2e-playground-coverage-failure
kind: feature
stage: drafting
tags: [testing, e2e-test, playground, portal]
parent: epic-e2e-playground-coverage
depends_on: [feature-e2e-playground-coverage-golden]
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Failure-mode e2e tests for the playground subsystem

## Brief

Failure-mode end-to-end coverage for the v0.4.0 playground subsystem.
Tests assert that the documented failure responses (429 rate-limit, 413/
pre-receive content-cap rejection, 401 bearer expiry, exit-1 reserved-slug
boot conflict) actually fire against the real portal binary in a
Testcontainers stack — not just against the unit-suite's stubbed clock
and stubbed storage.

Depends on `feature-e2e-playground-coverage-golden` because failure
tests reuse the patterns golden establishes: fixture composition,
bearer-header injection through the `binary` fixture, the
`dockerExec`-based assertion shape, and the `/test/clock-advance`
helper for time-bounded failure modes (bearer expiry).

## Child stories

This feature has 4 child stories, all carried over from the
`e2e-test-design --audit` run:

1. `e2e-audit-playground-rate-limit-abuse-cap` (High) — 4th
   `POST /api/playground/sessions` from the same IP within an hour gets
   a real 429, with the `Retry-After` header populated
2. `e2e-audit-playground-content-cap-pre-receive-enforcement` (High) —
   a push that pushes total repo size past 50 MiB is rejected at the
   real `pre-receive` hook (not just at the unit-level
   `prereceive.ValidateContentCap` function)
3. `e2e-audit-playground-bearer-expiry-hard-cap` (High) — uses
   `/test/clock-advance` to skip past hard-cap; subsequent bearer use
   returns the documented 401 with the right error envelope
4. `e2e-audit-playground-reserved-org-slug-boot-conflict` (Medium) —
   boots the portal binary with `--playground-enable` against a DB
   pre-seeded with a non-protected `playground` slug; asserts the
   binary exits 1 with the documented error message (goes in
   `tests/e2e/failure/config_and_deps_test.go` alongside existing
   boot-failure tests)

## Design status

Same as golden — the audit supplied sketches; e2e-test-design's job is
to lock the mock-boundary plan, surface ambiguities (e.g. how does the
rate-limit test reset between subtests without polluting shared portal
state?), and pre-mortem the suite.

## Next

`/agile-workflow:e2e-test-design feature-e2e-playground-coverage-failure`
once golden is at `stage: implementing` or beyond.
