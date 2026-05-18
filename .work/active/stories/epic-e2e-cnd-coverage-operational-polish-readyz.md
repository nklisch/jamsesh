---
id: epic-e2e-cnd-coverage-operational-polish-readyz
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

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Both tests are structurally sound. The golden test asserts all
acceptance criteria: 200 status, `application/json; charset=utf-8` Content-Type,
`status:"ready"`, non-empty checks array, and `ok:true` per check. The failure
test follows the toxiproxy `reset_peer` pattern correctly — pre-fault sanity
check, `require.Eventually` with the 3s/200ms poll matching the 2s probe timeout
plus 1s margin, and a final body-shape assertion confirming `status:"not_ready"`
and at least one `ok:false` check. No tautological assertions; every assertion
is against real HTTP responses and body shape. Package collision avoidance via
prefixed DSN helpers (`readyzExtractHost`, `readyzExtractDBName`) is correct.
SMTP omitted from the failure test is appropriate since only the DB probe path
is under test. No foundation-doc drift; no security or breaking-change concerns.

## Implementation notes

### Golden test (`tests/e2e/golden/readyz_healthy_test.go`)

Full stack: postgres + mailhog + portal. Uses `pg.ContainerDSN` and
`mh.ContainerSMTPHost`/`ContainerSMTPPort` so the portal container
reaches dependencies via Docker bridge IPs (host-mapped ports are not
reachable from inside Docker). Asserts HTTP 200, `application/json;
charset=utf-8` Content-Type, `status:"ready"`, non-empty checks array,
and `ok:true` on every check.

### Failure test (`tests/e2e/failure/readyz_db_down_test.go`)

Toxiproxy sits between the portal and Postgres containers. The proxy
listens on `0.0.0.0:5433` inside the toxiproxy container; the portal's
`DBDSN` points at `tp.ContainerIP:5433`. After a pre-fault 200 sanity
check, a `reset_peer` toxic (timeout:0) is injected; `require.Eventually`
polls for 503 within 3s (2s per-check timeout + 1s margin). A final
request asserts on the full body shape: `status:"not_ready"` and at least
one check with `ok:false`.

### Package collision avoidance

`config_and_deps_test.go` in the same `failure_test` package already
defines `toxiproxyCreateProxy`, `toxiproxyAddToxic`, and
`toxiproxyDeleteToxic` — so those helpers are available without
redeclaring them. DSN helpers are prefixed (`readyzExtractHost`,
`readyzExtractDBName`) to avoid collision with `extractHostFromDSN` and
`extractDBName` already declared in the same package.

### Build + vet

`go build ./golden/... ./failure/...` and `go vet ./golden/... ./failure/...`
both pass cleanly.
