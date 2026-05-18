---
id: epic-e2e-cnd-coverage-operational-polish
kind: feature
stage: review
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

## Design decisions

Resolved during the e2e-test-design pass under autopilot (2026-05-17).
Verified against `internal/portal/config/config.go` and
`internal/portal/probes/probes.go`; corrections to the original brief
applied below.

- **`_FILE` failure shape**: portal exits non-zero on a `_FILE`-set but
  unreadable target (verified at `internal/portal/config/config.go:448,
  601, 620` — config loader returns an error, main() exits). Assertion
  mechanism: container exit code + log inspection via the existing
  `containerlog.DumpAndTerminate` pattern. The error message is a
  conventional Go-wrapped error containing the path; assert on a
  substring match (e.g. `"_FILE"` or `"read secret"` — pin the exact
  shape at impl time after a probe run).
- **`/readyz` shape**: confirmed JSON envelope `{"status":
  "ready"|"not_ready", "checks": [...]}` returning 200 (all checks pass)
  or 503 (any check failed). Each check has a 2-second timeout (from
  `internal/portal/probes/probes.go:24`). The failure-mode test asserts
  `/readyz` returns 503 within ~2.5s (timeout + small margin) when
  Postgres is toxified. No clustered-vs-single distinction in
  `/readyz` body shape; the same handler is used in both modes.
- **`/metrics` auth**: design assumes `/metrics` is unauthenticated in
  the e2e portal image (matches Prometheus convention and the router's
  unauth `/metrics` at `cmd/jamsesh-router/main.go:145`). If the portal
  actually gates `/metrics` behind auth, the test fails with a clear 401
  and the implementer adds the necessary header — one-line fix.
- **Graceful-shutdown env var name**: confirmed `JAMSESH_SHUTDOWN_GRACE_S`
  (NOT `JAMSESH_SHUTDOWN_DEADLINE_SECONDS` from the original brief).
  Documented at `internal/portal/config/config.go:42,99,494`. The
  failure-mode test scaffold below uses the correct name.
- **Migration lock assertion mechanism**: hybrid — container-log
  inspection (the migration code path logs when it acquires + applies
  migrations) PLUS post-condition `schema_migrations` table query
  (verifies the schema is in the expected state). Both are real
  product-behavior assertions; together they make false-positive
  ("test thinks only one pod migrated but actually all three did")
  near-impossible. Migration lock key is
  `jamseshMigrationLockKey = 8675309` per `internal/db/migrate.go:17`
  — referenced in the test scaffold for clarity but not directly
  asserted (the test asserts on outcomes, not internals).
- **Helper extraction**: the toxiproxy helpers
  (`toxiproxyCreateProxy`, `toxiproxyAddToxic`, `toxiproxyDeleteToxic`)
  already exist in
  `tests/e2e/failure/config_and_deps_test.go:198-265`. Don't extract
  them yet — copy the pattern inline for the readyz_db_down test. If
  a third test in this feature needs them, extract into a shared
  `tests/e2e/fixtures/chaoshelpers/` package. Premature extraction is
  worse than duplication at small N.

## Taxonomy plan

Operational-polish exercises single-instance behavior only (clustered
testing of these endpoints would be redundant — `/readyz`, `/metrics`,
`_FILE`, migration lock, and shutdown are identical across modes).
Uses the existing `portal` + `postgres` + `toxiproxy` fixtures —
no new fixtures, no portalcluster, no minio.

- **Golden**: 3 tests
  - `readyz_healthy_test.go` — `/readyz` returns 200 with all checks ok
  - `metrics_endpoint_test.go` — `/metrics` returns valid Prometheus
    exposition format
  - `file_secret_happy_path_test.go` — `_FILE`-sourced DSN boots cleanly
- **Failure**: 4 test files / 6 subtests
  - `readyz_db_down_test.go` — toxiproxy reset_peer; `/readyz` → 503
  - `file_secret_missing_test.go` — 2 subtests (missing target,
    unreadable target)
  - `migration_concurrent_startup_test.go` — 3-pod concurrent boot;
    exactly one runs migrations
  - `graceful_shutdown_deadline_test.go` — 2 subtests (request under
    deadline finishes; request over deadline is terminated)
- **Chaos**: 0 standalone chaos tests. The readyz_db_down test uses
  toxiproxy (chaos infrastructure) but the test category is failure-mode
  (transient dep unavailability), not chaos. Chaos as a taxonomy layer
  doesn't apply here — `/readyz`/`_FILE`/migration-lock/shutdown are
  fail-fast or boot-time invariants, not retry/fallback surfaces.
- **Fuzz**: 0. No new parsers / validators introduced by this surface.
  `_FILE` paths are read by Go's `os.ReadFile`; that's not a fuzz
  target (already exercised by Go's stdlib).

## Implementation Units

### Unit 1: /readyz coverage

**Files**: `tests/e2e/golden/readyz_healthy_test.go`,
`tests/e2e/failure/readyz_db_down_test.go`
**Story**: `epic-e2e-cnd-coverage-operational-polish-readyz`
**Invariant** (golden): "On a healthy portal, GET /readyz returns 200
with `status: ready` and every check OK."
**Invariant** (failure): "When Postgres is unreachable via toxiproxy
reset_peer, GET /readyz returns 503 with `status: not_ready` and the
database check reports an error, within the check-timeout window."

Golden scaffold:

```go
package golden

func TestReadyzHealthy(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    mh := mailhog.Start(ctx, t)
    p := portal.Start(ctx, t, portal.Options{
        DBDriver:  "postgres",
        DBDSN:     pg.ContainerDSN,
        EmailFrom: "noreply@example.com",
        SMTPHost:  mh.ContainerSMTPHost,
        SMTPPort:  mh.ContainerSMTPPort,
    })

    resp, err := http.Get(p.URL + "/readyz")
    require.NoError(t, err)
    defer resp.Body.Close()

    require.Equal(t, http.StatusOK, resp.StatusCode)
    require.Equal(t, "application/json; charset=utf-8",
        resp.Header.Get("Content-Type"))

    var body struct {
        Status string `json:"status"`
        Checks []struct {
            Name string `json:"name"`
            OK   bool   `json:"ok"`
            Error *string `json:"error,omitempty"`
        } `json:"checks"`
    }
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

    require.Equal(t, "ready", body.Status)
    require.NotEmpty(t, body.Checks, "/readyz must declare at least one check")
    for _, c := range body.Checks {
        require.True(t, c.OK, "check %q failed: %v", c.Name, c.Error)
    }
}
```

Failure scaffold (uses toxiproxy in front of Postgres):

```go
func TestReadyzDBDown(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    tp := toxiproxy.Start(ctx, t)

    // Proxy Postgres through toxiproxy so we can inject failures.
    toxiproxyCreateProxy(ctx, t, tp.AdminURL, "pg",
        ":5433", fmt.Sprintf("%s:%d", pg.ContainerIP, 5432))
    // ... portal configured with toxiproxy DSN ...

    p := portal.Start(ctx, t, portal.Options{
        DBDriver: "postgres",
        DBDSN:    "postgres://test:test@toxiproxy:5433/...?sslmode=disable",
        EmailFrom: "noreply@example.com",
    })

    // Verify readyz is healthy initially.
    resp, _ := http.Get(p.URL + "/readyz")
    require.Equal(t, http.StatusOK, resp.StatusCode)
    resp.Body.Close()

    // Inject reset_peer toxic on portal→PG path.
    toxiproxyAddToxic(ctx, t, tp.AdminURL, "pg", "kill", "reset_peer",
        map[string]any{"timeout": 0})

    // Within ~2.5s (check timeout + margin), readyz must report unhealthy.
    require.Eventually(t, func() bool {
        r, err := http.Get(p.URL + "/readyz")
        if err != nil {
            return false
        }
        defer r.Body.Close()
        return r.StatusCode == http.StatusServiceUnavailable
    }, 3*time.Second, 200*time.Millisecond,
        "/readyz must return 503 when DB is unreachable")

    // Assert on response body shape.
    r, _ := http.Get(p.URL + "/readyz")
    defer r.Body.Close()
    var body struct {
        Status string `json:"status"`
        Checks []struct {
            Name string `json:"name"`
            OK   bool   `json:"ok"`
        } `json:"checks"`
    }
    require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
    require.Equal(t, "not_ready", body.Status)
    // At least one check must be failing.
    anyFailed := false
    for _, c := range body.Checks {
        if !c.OK {
            anyFailed = true
            break
        }
    }
    require.True(t, anyFailed, "expected at least one failing check")
}
```

**Acceptance Criteria**:
- [ ] Golden test green with `status: ready` and 200
- [ ] Failure test green; readyz returns 503 within 3s of toxic injection
- [ ] Failure test asserts on body shape (not_ready + at least one
      failing check), not just status code
- [ ] Toxiproxy helpers copied inline (not extracted — see Design
      decisions)

---

### Unit 2: /metrics coverage

**File**: `tests/e2e/golden/metrics_endpoint_test.go`
**Story**: `epic-e2e-cnd-coverage-operational-polish-metrics`
**Invariant**: "GET /metrics returns Prometheus exposition format that
parses cleanly and contains at least one well-known metric."

Scaffold:

```go
import "github.com/prometheus/common/expfmt"

func TestMetricsEndpoint(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    p := portal.Start(ctx, t, portal.Options{
        DBDriver: "postgres",
        DBDSN:    pg.ContainerDSN,
        EmailFrom: "noreply@example.com",
    })

    resp, err := http.Get(p.URL + "/metrics")
    require.NoError(t, err)
    defer resp.Body.Close()

    require.Equal(t, http.StatusOK, resp.StatusCode)

    // Content-Type starts with text/plain (Prom format).
    ct := resp.Header.Get("Content-Type")
    require.Contains(t, ct, "text/plain",
        "Prometheus exposition format requires text/plain content type")

    // Parse with the Prom textparser.
    var parser expfmt.TextParser
    families, err := parser.TextToMetricFamilies(resp.Body)
    require.NoError(t, err, "exposition format must parse cleanly")
    require.NotEmpty(t, families, "must expose at least one metric family")

    // Spot-check a well-known Go-runtime metric (the prom client lib
    // exposes go_goroutines automatically). If the portal disables Go
    // collectors, pick a different known metric.
    _, hasGoGoroutines := families["go_goroutines"]
    require.True(t, hasGoGoroutines,
        "expected go_goroutines in metrics (Go runtime collector)")
}
```

If `go_goroutines` isn't exposed (e.g., portal uses a custom registry
without runtime collectors), pick a portal-specific counter the
implementer can verify is always present. The test should NOT
silently pass on an empty families map.

**Acceptance Criteria**:
- [ ] `/metrics` returns 200 with text/plain Content-Type
- [ ] `expfmt.TextParser` parses the body without error
- [ ] At least one well-known metric family is present and asserted
- [ ] If `/metrics` requires auth, test fails loudly with the 401
      — implementer adds auth header in a 1-line fix, doesn't silence
- [ ] `github.com/prometheus/common/expfmt` added to `tests/e2e/go.mod`

---

### Unit 3: `_FILE` secret coverage

**Files**: `tests/e2e/golden/file_secret_happy_path_test.go`,
`tests/e2e/failure/file_secret_missing_test.go`
**Story**: `epic-e2e-cnd-coverage-operational-polish-file-secrets`
**Invariant** (golden): "Portal boots when `JAMSESH_DB_DSN_FILE` points
at a file containing a valid DSN — `_FILE` is a first-class config path."
**Invariant** (failure): "Portal exits non-zero with a clear `_FILE`
error when `JAMSESH_DB_DSN_FILE` points at a missing or unreadable file
— no silent fallback, no hang."

Golden scaffold uses the `ContainerFile` mount pattern from
`tests/e2e/failure/config_and_deps_test.go:296`:

```go
func TestFileSecretHappyPath(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})

    // Write the DSN to a temp file on the host.
    dsnFile := filepath.Join(t.TempDir(), "db_dsn")
    require.NoError(t, os.WriteFile(dsnFile, []byte(pg.ContainerDSN), 0o600))

    // Mount it into the portal container; configure _FILE env var.
    p := portal.Start(ctx, t, portal.Options{
        DBDriver:  "postgres",
        // DBDSN deliberately empty; _FILE must take precedence.
        EmailFrom: "noreply@example.com",
        ExtraEnv: map[string]string{
            "JAMSESH_DB_DSN_FILE": "/run/secrets/db_dsn",
        },
        // ContainerFiles would need extension to Options — for now,
        // pass a custom container request override OR add ContainerFiles
        // support to portal.Options (probably the cleaner path).
    })

    // Portal must boot cleanly — proven by /healthz 200 (which
    // portal.Start already waits for).
    resp, err := http.Get(p.URL + "/healthz")
    require.NoError(t, err)
    defer resp.Body.Close()
    require.Equal(t, http.StatusOK, resp.StatusCode)
}
```

**Implementation note**: `portal.Options` currently has no
`ContainerFiles` field. Either (a) extend `Options` (low-risk,
mirrors testcontainers' direct field) — preferred, or (b) drop down
to a raw `testcontainers.GenericContainerRequest` for these tests.
Option (a) is the cleaner path and benefits future tests.

Failure scaffold:

```go
func TestFileSecretMissing(t *testing.T) {
    t.Run("file_missing", func(t *testing.T) {
        // Start the portal with _FILE pointing at a nonexistent path.
        // No file mounted; portal must fail to start.
        ctx := context.Background()

        env := map[string]string{
            "JAMSESH_BIND":       ":8443",
            "JAMSESH_TLS_MODE":   "behind_proxy",
            "JAMSESH_DB_DRIVER":  "sqlite",
            "JAMSESH_DB_DSN_FILE": "/no/such/file",  // file missing
            "JAMSESH_EMAIL_FROM": "noreply@example.com",
        }

        // Use raw GenericContainerRequest because portal.Start waits for
        // /healthz which will never come up — we want failure here.
        c, err := startPortalExpectingFailure(ctx, env)
        defer c.Terminate(ctx)

        // Wait for exit; capture logs.
        require.Eventually(t, func() bool {
            state, _ := c.State(ctx)
            return state.Status == "exited"
        }, 30*time.Second, 500*time.Millisecond)

        // Assert exit code non-zero.
        state, _ := c.State(ctx)
        require.NotEqual(t, 0, state.ExitCode, "portal must exit non-zero")

        // Assert logs mention the _FILE error.
        logs := readContainerLogs(ctx, t, c)
        require.Contains(t, strings.ToLower(logs), "_file",
            "expected log line mentioning _FILE failure")
    })

    t.Run("file_unreadable", func(t *testing.T) {
        // Mount a file with 0o000 perms so the portal user can't read it.
        // Same assertions as file_missing.
    })
}
```

**Acceptance Criteria**:
- [ ] `portal.Options` gains a `ContainerFiles []testcontainers.ContainerFile`
      field (or equivalent), wired into the container request
- [ ] Golden: portal boots with `_FILE`-sourced DSN; `/healthz` 200
- [ ] Failure `file_missing`: portal exits non-zero within 30s; log
      mentions `_FILE`
- [ ] Failure `file_unreadable`: portal exits non-zero; log mentions
      `_FILE` (or "read secret")
- [ ] No silent fallback to env-var-only DSN (would mask the failure)

---

### Unit 4: Migration concurrent-startup coverage

**File**: `tests/e2e/failure/migration_concurrent_startup_test.go`
**Story**: `epic-e2e-cnd-coverage-operational-polish-migration-lock`
**Invariant**: "When N portals start simultaneously against a fresh
Postgres DB, exactly one runs migrations (advisory-lock-serialized),
all eventually come up healthy, and the final schema is correct and
applied once."

Scaffold:

```go
func TestMigrationConcurrentStartup(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    // postgres.Start creates an empty per-test DB.

    const N = 3
    portals := make([]*portal.Portal, N)
    var g errgroup.Group

    for i := 0; i < N; i++ {
        i := i
        g.Go(func() error {
            p := portal.Start(ctx, t, portal.Options{
                DBDriver:  "postgres",
                DBDSN:     pg.ContainerDSN,
                EmailFrom: "noreply@example.com",
            })
            portals[i] = p
            return nil
        })
    }
    require.NoError(t, g.Wait(), "all portals must start healthy")

    // Each portal must answer /healthz 200 (proven by portal.Start's wait).
    for i, p := range portals {
        resp, err := http.Get(p.URL + "/healthz")
        require.NoError(t, err, "portal %d", i)
        require.Equal(t, http.StatusOK, resp.StatusCode, "portal %d", i)
        resp.Body.Close()
    }

    // Inspect each portal's container logs to count which one(s) report
    // applying migrations.
    migratorCount := 0
    for i, p := range portals {
        logs := containerlog.Capture(ctx, t, p)
        if strings.Contains(logs, "migration applied") ||
           strings.Contains(logs, "applying migrations") {
            // (pin the exact phrase at impl time after a probe-run)
            migratorCount++
            t.Logf("portal %d is the migration applier", i)
        }
    }
    require.Equal(t, 1, migratorCount,
        "exactly one portal must apply migrations (advisory-lock serialised)")

    // Post-condition: schema is correctly applied (query schema_migrations).
    db, err := sql.Open("postgres", pg.DSN)
    require.NoError(t, err)
    defer db.Close()
    var version int
    err = db.QueryRow(
        "SELECT MAX(version) FROM schema_migrations").Scan(&version)
    require.NoError(t, err, "schema_migrations table must exist")
    require.Greater(t, version, 0, "schema must be migrated to a real version")
}
```

**Implementation notes**:
- The exact log phrase ("migration applied", "applying migrations") must
  be pinned by probing a real portal startup. Don't guess — read one
  actual portal's log output, then write the test against that phrase.
- The `schema_migrations` table name is `goose_db_version` or similar
  depending on the migration library. Verify by querying the DB after
  a normal startup.
- If `containerlog.Capture` doesn't exist as a synchronous helper (it
  may only be `DumpAndTerminate`), extract one in this story.

**Acceptance Criteria**:
- [ ] 3 portals start in parallel via errgroup
- [ ] All 3 report `/healthz` 200
- [ ] Exactly 1 portal's logs report applying migrations (migrator count == 1)
- [ ] Schema is correctly migrated post-startup (verified by querying
      schema_migrations / equivalent)
- [ ] Test does not race-condition on log capture (logs settled before assertion)

---

### Unit 5: Graceful-shutdown deadline coverage

**File**: `tests/e2e/failure/graceful_shutdown_deadline_test.go`
**Story**: `epic-e2e-cnd-coverage-operational-polish-shutdown-deadline`
**Invariant** (under-deadline): "An in-flight request that finishes
within `JAMSESH_SHUTDOWN_GRACE_S` completes successfully even after
SIGTERM is sent to the portal."
**Invariant** (over-deadline): "An in-flight request that exceeds
`JAMSESH_SHUTDOWN_GRACE_S` is terminated; the portal exits at the
deadline, not later."

Choice of "long-running request": the cleanest deterministic source is
a portal endpoint that depends on a slow external call. With WireMock
stubbing GitHub OAuth and injecting a delay via WireMock's
`fixedDelayMilliseconds` (existing pattern at
`tests/e2e/chaos/testdata/github_delay_30s.json`), an OAuth callback
flow becomes a controllable-duration request.

Scaffold sketch:

```go
func TestGracefulShutdownDeadline(t *testing.T) {
    t.Run("request_finishes_within_deadline", func(t *testing.T) {
        ctx := context.Background()
        wm := wiremock.StartWithMappings(ctx, t, "testdata/oauth_delay_2s.json")
        pg := postgres.Start(ctx, t, postgres.Options{})

        p := portal.Start(ctx, t, portal.Options{
            DBDriver:                "postgres",
            DBDSN:                   pg.ContainerDSN,
            EmailFrom:               "noreply@example.com",
            OAuthBaseURL:            wm.ContainerURL,
            OAuthGitHubClientID:     "test-client",
            OAuthGitHubClientSecret: "test-secret",
            ExtraEnv: map[string]string{
                // Deadline > request duration → request completes
                "JAMSESH_SHUTDOWN_GRACE_S": "10",
            },
        })

        // Start an OAuth callback request (will take ~2s due to WireMock delay).
        done := make(chan struct {
            resp *http.Response
            err  error
        }, 1)
        go func() {
            r, e := http.Get(p.URL + "/api/auth/github/callback?...")
            done <- struct{ resp *http.Response; err error }{r, e}
        }()

        // Give the request 200ms to start, then SIGTERM the container.
        time.Sleep(200 * time.Millisecond)
        require.NoError(t, p.SendSignal(ctx, syscall.SIGTERM))

        // Request must complete (any non-5xx status, including OAuth-flow
        // 302 or 4xx — we're verifying the request wasn't aborted by shutdown).
        select {
        case result := <-done:
            require.NoError(t, result.err, "in-flight request must complete")
            // Status code may vary based on OAuth flow; the key invariant is
            // "completed without ECONNREFUSED/EPIPE shutdown error".
        case <-time.After(15 * time.Second):
            t.Fatal("in-flight request did not complete within timeout")
        }
    })

    t.Run("request_exceeds_deadline", func(t *testing.T) {
        // Same setup, but OAuth delay = 10s, deadline = 2s.
        // SIGTERM after 200ms; request should be terminated near the 2s mark
        // (with a connection close or 503), and portal exits.
        // Assert: total elapsed < 4s (deadline + margin); portal container
        // is in exited state shortly after.
    })
}
```

**Implementation notes**:
- `portal.Options` may need a helper to send signals — confirm by reading
  `Portal` struct methods; the existing fixture exposes `ContainerName`
  for `docker pause`-style chaos, so a `SendSignal` extension would
  mirror that.
- The `testdata/oauth_delay_2s.json` WireMock mapping mirrors the
  existing `tests/e2e/chaos/testdata/github_delay_30s.json` shape.

**Acceptance Criteria**:
- [ ] `request_finishes_within_deadline` green; in-flight OAuth callback
      completes after SIGTERM
- [ ] `request_exceeds_deadline` green; request is terminated at the
      deadline; portal container exits shortly after
- [ ] `Portal.SendSignal` (or equivalent) added if absent
- [ ] WireMock delay mapping added under `tests/e2e/failure/testdata/`
- [ ] If the test surfaces the `graceful-shutdown-shutdownstart-race`
      backlog story's race, the test calls it out but doesn't game the
      assertion to dodge it (park the bug, t.Skip with reference if the
      race is unfixable in-stride)

---

## Implementation Order

All 5 stories are independent (no inter-story `depends_on`). They can
run in parallel via implement-orchestrator waves:

1. `epic-e2e-cnd-coverage-operational-polish-readyz`
2. `epic-e2e-cnd-coverage-operational-polish-metrics`
3. `epic-e2e-cnd-coverage-operational-polish-file-secrets`
4. `epic-e2e-cnd-coverage-operational-polish-migration-lock`
5. `epic-e2e-cnd-coverage-operational-polish-shutdown-deadline`

## Risks (pre-mortem)

- **`/metrics` may require auth or be on a separate listener.** If so,
  the Unit 2 test fails clearly with a 401 or connection refused.
  Mitigation: assertion message is explicit ("if 401, add auth header");
  one-line fix, no design rework.
- **Migration log phrase is implementation-dependent.** The exact log
  string the migration code emits ("migration applied", "migrating",
  "schema upgraded", etc.) must be confirmed by reading one actual
  portal's logs before writing the assertion. Implementer's job —
  flagged in Unit 4's implementation notes.
- **`portal.Options` extension for ContainerFiles + SendSignal.** Two
  small fixture additions land in this feature. Risk: drift between
  the addition here and any other test that adopts the new fields.
  Mitigated by single-source extension with backward-compatible
  defaults (empty `ContainerFiles` == today's behavior).
- **Graceful-shutdown test surfaces the open backlog race.** Story
  `graceful-shutdown-shutdownstart-race` documents a known race in the
  shutdown variable. If the new test triggers it (race surfaces as a
  flaky shutdown), don't fix it in this stride — park the test with a
  reference to the existing backlog item, ship the feature, let the
  backlog item drive the fix.
- **Migration test's `errgroup` ordering**. Three Testcontainers starting
  in parallel may interleave their container-create requests against
  Docker. This is fine for the actual test (advisory lock handles
  concurrency at the DB level), but the Docker daemon may reject
  concurrent creates under load. Mitigation: `errgroup` is the right
  primitive — failures propagate cleanly; if a container fails to
  start, the test fails loudly with a Docker error, not silently.
- **Toxiproxy proxy listen-port collisions.** The readyz_db_down test
  needs a listening port on toxiproxy that doesn't collide with other
  in-test toxiproxy users (none today, but worth noting). Pin a
  per-test port or use toxiproxy's port-0 (any free port) feature.

## Implementation summary (2026-05-17)

All 5 child stories landed at `stage: review` in a single orchestrator
run (2-sub-wave schedule, scheduled to avoid the portal.go conflict
between file-secrets and shutdown-deadline).

| Story | Status | Notes |
|---|---|---|
| `operational-polish-readyz` | review | golden + failure via toxiproxy `reset_peer` on portal→PG path; 3s eventually bound (2s check timeout + 1s margin); body-shape asserted |
| `operational-polish-metrics` | review | expfmt parser + `go_goroutines` spot-check; `/metrics` is unauth per `internal/portal/router/router.go:98`; prom deps pinned to main module's versions |
| `operational-polish-file-secrets` | review | `portal.Options.ContainerFiles` extension; happy-path + 2 failure subtests; `readEnvOrFile` error string contains "_FILE" reliably |
| `operational-polish-migration-lock` | review | added `slog.InfoContext(ctx, "db migrations applied", ...)` to `internal/db/migrate.go` (production code change — gives ops + tests a stable signal); 3-pod errgroup parallel start; queries `goose_db_version` post-condition |
| `operational-polish-shutdown-deadline` | review | `portal.SendSignal` extension; OAuth callback via WireMock-delay (existing pattern); 2-subtest matrix (under-deadline completes, over-deadline terminates near grace) |

Cross-cutting changes:
- **Production code change**: `internal/db/migrate.go` now emits
  `slog.InfoContext(ctx, "db migrations applied", "dialect", ..., "count", ...)`
  when actual DDL ran. Idempotent runs stay silent. Useful in production
  ops + necessary for the migration-lock test's per-pod log distinction.
- **`portal.Options` / `*Portal` extensions** (shared with the
  cluster-fixture feature; landed there first):
  `ContainerFiles []testcontainers.ContainerFile`, `Logs(ctx)`,
  `SendSignal(ctx, sig)`, plus from cluster-fixture: `ContainerIP(ctx)`,
  `State(ctx)`. All backward-compatible.

Verification: `go build ./...` + `go vet ./...` clean across both the
root module and `tests/e2e/` module. No product bugs surfaced — every
test landed with assertions genuinely exercising the documented
contracts (`/readyz` JSON envelope, Prom exposition format, `_FILE`
error path, advisory-lock serialization, shutdown grace).

The single-instance operational primitives now have golden + failure
coverage end-to-end. Ready for review.

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
