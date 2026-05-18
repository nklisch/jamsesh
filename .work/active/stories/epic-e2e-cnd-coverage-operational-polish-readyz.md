---
id: epic-e2e-cnd-coverage-operational-polish-readyz
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

# `/readyz` coverage — golden + failure

## Scope

Two test files asserting on the `/readyz` JSON envelope (verified shape
from `internal/portal/probes/probes.go`: `{status: ready|not_ready,
checks: [{name, ok, error?}]}`; HTTP 200 vs 503 split; 2s per-check
timeout).

- `tests/e2e/golden/readyz_healthy_test.go` — full stack up, `/readyz`
  returns 200 with all checks OK.
- `tests/e2e/failure/readyz_db_down_test.go` — toxiproxy `reset_peer`
  on Postgres path; `/readyz` returns 503 within ~3s.

## Files

- `tests/e2e/golden/readyz_healthy_test.go`
- `tests/e2e/failure/readyz_db_down_test.go`

## Acceptance criteria

- [ ] Golden: 200 status, `application/json; charset=utf-8`
      Content-Type, body parses to `{status:"ready", checks:[...]}`,
      every check has `ok: true`, body has at least one check declared
- [ ] Failure: starts with healthy readyz (200), then injects
      reset_peer; within 3s `/readyz` returns 503 with
      `status: not_ready`, at least one check has `ok: false`
- [ ] Toxiproxy helpers copied inline from
      `tests/e2e/failure/config_and_deps_test.go:198-265` (don't
      extract — see parent feature's Design decisions)
- [ ] No mock-invocation asserts; every assertion is against the
      real HTTP response and body shape

## Test integrity (from parent epic)

- If `/readyz` returns 200 when DB is actually down, that's a real bug
  (K8s would keep routing traffic to a broken pod). Park via
  `/agile-workflow:park` with severity Critical; t.Skip with backlog id.
- Don't loosen the failure-test timeout to "make it pass" — the 2s
  check-timeout + small margin is the documented contract.
- Don't assert just on status code; the body shape is part of the
  contract (Prometheus, alerting, and ops dashboards parse it).

## References

- Parent feature body, Unit 1 — full scaffold
- `internal/portal/probes/probes.go` — verified response shape
- `tests/e2e/failure/config_and_deps_test.go:198-265,517-560` —
  toxiproxy `reset_peer` pattern to mirror
- `tests/e2e/fixtures/toxiproxy/` — Toxiproxy fixture API
