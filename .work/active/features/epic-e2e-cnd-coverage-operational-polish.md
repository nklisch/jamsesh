---
id: epic-e2e-cnd-coverage-operational-polish
kind: feature
stage: drafting
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E CND Coverage — Operational Polish

## Brief

The single-instance + clustered cloud-operability primitives landed by
`epic-cloud-native-deploy-operational-polish` have **no e2e coverage**.
The existing scaffolding tests hit `/healthz` only; nothing tests
`/readyz` (deeper liveness/readiness with DB ping + lease-pool health),
`/metrics` (Prometheus exposition), `_FILE` secret env-var variants
(e.g. `JAMSESH_DB_DSN_FILE`), the migration advisory lock (concurrent-
startup safety), or the configurable graceful-shutdown deadline.

This feature is independent of every other CND-coverage feature — it
exercises the single-instance portal fixture only, no clustered fixture
required. It can run in parallel with `cluster-fixture` from day one.

The five surfaces share two characteristics:
1. They're operational invariants that K8s / Cloud Run / Fly directly
   depend on (a `/readyz` that returns 200 when the DB is down is a real
   production hazard — K8s keeps routing to a broken pod).
2. They're testable with the existing `tests/e2e/fixtures/portal/`
   fixture plus the existing Postgres + Toxiproxy fixtures.

## Audit findings addressed

- **F5 (High, missing-taxonomy-layer golden+failure)** — `/readyz` endpoint
  not tested. `tests/e2e/scaffolding/healthz_test.go:66` and `portal_image_
  test.go:65` hit `/healthz` only. Zero references to `/readyz`.
- **F6 (High, missing-taxonomy-layer golden)** — `/metrics` Prometheus
  endpoint not tested. Zero references to `/metrics` or `prometheus` in
  `tests/e2e/`.
- **F7 (High, missing-taxonomy-layer failure)** — `_FILE` secret env-var
  variants not tested. `tests/e2e/failure/config_and_deps_test.go:347-433`
  covers direct env vars but not `_FILE` indirection.
- **F8 (High, missing-taxonomy-layer failure/chaos)** — Migration advisory
  lock contention has no concurrent-startup test. Tests bring up exactly
  one portal per test.
- **F9 (Medium, missing-taxonomy-layer failure)** — Graceful-shutdown
  deadline has no failure coverage. Cross-reference: open backlog story
  `graceful-shutdown-shutdownstart-race` flags the absence of test
  exercise on the shutdown path as a reason a race went unnoticed.

## Scope

### Tests to add

1. **`tests/e2e/golden/readyz_healthy_test.go`** — `GET /readyz` on a
   fully-healthy portal returns 200 and an OK body. Single-instance
   fixture; one subtest.

2. **`tests/e2e/failure/readyz_db_down_test.go`** — Apply Toxiproxy
   `reset_peer` toxic to the portal↔Postgres path (existing pattern from
   `tests/e2e/failure/config_and_deps_test.go:522`). Within an SLO,
   `/readyz` must return non-200. Critical for K8s readiness probes.

3. **`tests/e2e/golden/metrics_endpoint_test.go`** — `GET /metrics` returns
   `Content-Type: text/plain; version=0.0.4`, body parses with
   `github.com/prometheus/common/expfmt.TextParser`, contains at least one
   well-known counter (e.g., `http_requests_total` or `go_goroutines`).
   Lightweight; one subtest.

4. **`tests/e2e/failure/file_secret_missing_test.go`** — Two subtests:
   - `db_dsn_file_target_missing` — set `JAMSESH_DB_DSN_FILE=/no/such/file`
     (no `JAMSESH_DB_DSN`), assert portal exits non-zero with a log line
     containing the path and a `_FILE` error string.
   - `db_dsn_file_traversal_or_unreadable` — set
     `JAMSESH_DB_DSN_FILE=/etc/shadow` (unreadable to the portal user),
     assert fail-fast with a sanitized error.
   - Reuse `testcontainers.ContainerFile` mounting pattern from
     `tests/e2e/failure/config_and_deps_test.go:296`.

5. **`tests/e2e/golden/file_secret_happy_path_test.go`** — set
   `JAMSESH_DB_DSN_FILE=/run/secrets/db_dsn` (mounted via
   `ContainerFile`) with valid DSN content; portal boots and `/healthz`
   returns 200. Proves `_FILE` is a first-class supported config path.

6. **`tests/e2e/failure/migration_concurrent_startup_test.go`** — Start
   3 portals simultaneously against a fresh Postgres DB (no schema yet).
   Assert: all three eventually report `/healthz` 200; only one ran
   migrations (verified by log inspection or by counting expected schema
   versions in `schema_migrations` table); the final schema is correct
   and complete. Real Postgres advisory locks are the spec — no
   in-process mocks possible.

7. **`tests/e2e/failure/graceful_shutdown_deadline_test.go`** — Two
   subtests:
   - `request_finishes_within_deadline` — start a long-but-bounded REST
     request (e.g., a session-list query with sleep injected via
     test-clock or wiremock-mediated dependency), SIGTERM the portal
     mid-flight; assert the request completes with 2xx and the portal
     exits cleanly within `JAMSESH_SHUTDOWN_DEADLINE_SECONDS + small
     margin`.
   - `request_exceeds_deadline` — set a tight deadline, start a
     deliberately-too-long request, SIGTERM; assert the in-flight
     request gets a 503 (or connection close) and the portal exits at
     the deadline (not later).
   - Cross-validates with backlog story
     `graceful-shutdown-shutdownstart-race`; if the race surfaces during
     this test, log it and pin a comment / `skip` per test-integrity
     rules. Do not silently fix.

### Helpers / fixtures

- May need a tiny `_FILE` mount helper in `tests/e2e/fixtures/portal/` to
  reduce per-test boilerplate around `ContainerFile`. Design-pass call.
- The migration test may need a "wait for all N portals to settle"
  helper. Design-pass call.

## Mock-boundary plan

| External dep                  | Service-level mock | Notes |
|-------------------------------|--------------------|-------|
| Postgres (single + multi-pod) | Existing `postgres` fixture | Reuse |
| Postgres latency injection    | Existing `toxiproxy` fixture | Reuse F5 pattern |
| File-based secrets            | `testcontainers.ContainerFile` | Already in use at `config_and_deps_test.go:296` |
| Prometheus format parsing     | `prometheus/common/expfmt` (real lib) | In-process parser is OK — it's the test asserting on the real portal's output, not a mock of the portal |

No in-process mocks of the portal or DB.

## Open questions for design

- **`_FILE` log-line assertion shape.** Implementation surfaces the
  `_FILE` failure how? Stderr? Stdout? `log/slog` JSON? Will dictate the
  assertion mechanism (`containerlog` fixture vs. exit-code-only). Confirm
  in design pass by reading log output from a deliberately-broken
  startup — without reading implementation source.
- **`/readyz` semantics in clustered vs single-instance mode.** Does
  `/readyz` consider lease-pool health in single-instance? If yes, F5's
  toxiproxy-DB chaos test should assert the right failure mode for both
  shapes. If single-instance `/readyz` only checks DB, that's simpler.
  Confirm by reading the `/readyz` response body shape (a probe call to
  a real portal), not by reading the source.
- **`/metrics` authentication.** Is `/metrics` open or behind auth in
  production? If behind auth, the golden test needs to authenticate
  (and the failure test should assert unauthenticated returns 401/403).
- **Graceful-shutdown deadline env var name.** SPEC.md should have it;
  design pass confirms canonical name (`JAMSESH_SHUTDOWN_DEADLINE_SECONDS`
  is a guess from the audit context).
- **Migration log-inspection assertion.** Counting "ran migrations" can
  be done by:
  - (a) log-line grep on each pod
  - (b) Postgres `schema_migrations` table row count, before/after
  - (c) deliberate schema mutation that only one pod could have done
  Design pass picks one based on what the implementation produces.

## Acceptance criteria

- [ ] `tests/e2e/golden/readyz_healthy_test.go` green; asserts on response
      status + body shape
- [ ] `tests/e2e/failure/readyz_db_down_test.go` green; `/readyz`
      reports non-200 within SLO when Postgres is toxified
- [ ] `tests/e2e/golden/metrics_endpoint_test.go` green; format parses
      via `expfmt`, well-known counter present
- [ ] `tests/e2e/failure/file_secret_missing_test.go` green; both
      subtests assert specific error shape (not just non-zero exit)
- [ ] `tests/e2e/golden/file_secret_happy_path_test.go` green; portal
      boots fully with `_FILE`-sourced DSN
- [ ] `tests/e2e/failure/migration_concurrent_startup_test.go` green;
      proves exactly one of N pods runs migrations, all settle healthy
- [ ] `tests/e2e/failure/graceful_shutdown_deadline_test.go` green; both
      under-deadline and over-deadline subtests pass
- [ ] No new in-process mocks introduced
- [ ] Any production bugs surfaced (e.g. the
      `graceful-shutdown-shutdownstart-race` already-flagged race) are
      handled per test-integrity rules: park if not fixed inline, attach
      `skip`/`xfail` with backlog-id reference

## Test integrity (from parent epic)

- **Park production bugs, don't hide them.** If `/readyz` returns 200
  when the DB is down, `/metrics` returns wrong format, `_FILE` silently
  ignores a missing file, or the migration advisory lock fails to serialize
  startup — park the bug via `/agile-workflow:park`. Land the failing test
  with `t.Skip("XXX: backlog item id; reason")` or `t.Fatal` + clear
  message. The failing test is documentation.
- **Fix bad tests in-session.** If a stale assertion in an adjacent test
  surfaces during this work, repair it.
- **Never game an assertion.** No `require.NoError(t, err)` on a discarded
  err that should have failed the test; no asserting on whatever response
  the portal happens to return when the spec is clear about what it
  should return.

## Non-goals

- PG pool tuning behavioral tests (the env vars exist; pool sizing
  behavior under load is perf territory, not coverage)
- Multi-mode (single + clustered) readyz parity matrix — falls out
  naturally if both shapes are tested but not a primary goal here

## Next

`/agile-workflow:e2e-test-design epic-e2e-cnd-coverage-operational-polish`
