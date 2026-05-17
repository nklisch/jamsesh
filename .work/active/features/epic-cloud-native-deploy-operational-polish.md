---
id: epic-cloud-native-deploy-operational-polish
kind: feature
stage: implementing
tags: [infra, portal]
parent: epic-cloud-native-deploy
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Cloud-Native Deploy — Operational Polish

## Epic context

- Parent epic: `epic-cloud-native-deploy`
- Position in epic: phase-1 foundation feature. Ships standalone with
  standalone value (improves any deploy, single-instance or future
  clustered). Routing-layer and lease-fencing both depend on this for
  `/readyz` and the migration-lock primitive.

## Foundation references

- `docs/SPEC.md` — "Deployment shape" (current single-binary shape this
  feature reinforces) and "Hard constraints / Self-host-capable" (the
  must-not-regress invariant).
- `docs/SELF_HOST.md` — §1 Install, §2 Configuration (env vars table
  this feature extends), §8 Monitoring (mentions `/metrics` as future
  work — this feature delivers it), §9 Upgrade procedure (graceful
  shutdown is the missing primitive there).
- `internal/portal/config/config.go` — existing config pattern this
  feature extends (env-overlay-over-YAML, `JAMSESH_<SECTION>_<KEY>`
  naming).
- `internal/portal/server/server.go` — existing 25-second graceful-
  shutdown skeleton this feature firms up.

## Brief

The cloud-operability primitives that make the existing single-instance
portal deploy cleanly on any modern cloud platform. Ships as phase 1 of
`epic-cloud-native-deploy` and stands on its own — none of the
clustered-mode features depend on this beyond what's listed here.

Everything in this feature is also valuable for the clustered-mode
features (phase 2 of the epic), but the converse is not true: an operator
can take just this feature and get a noticeably smoother experience
deploying jamsesh on Cloud Run (min=max=1), Fly, Railway, a single VM,
or k8s with a `PersistentVolumeClaim`, without committing to the
clustered architecture.

## Scope

In:
- `/readyz` endpoint, separate from `/healthz`, that probes DB
  connectivity and storage-root accessibility. Used by k8s readiness
  probes, Cloud Run startup probes, and the future routing layer.
- `/metrics` Prometheus endpoint exposing standard process metrics plus
  portal-specific counters (HTTP request rates, push counts, auto-merger
  results, event-log throughput). Currently listed as "future" in
  `docs/SELF_HOST.md` §8.
- `JAMSESH_PORTAL_URL` already exists in `internal/portal/config/`. This
  feature audits every place where the portal constructs an externally-
  visible URL (OAuth callback, MCP discovery, future router probe URL)
  and ensures all of them honor the configured `PortalURL` instead of
  deriving from `Bind`. Documents the var in `docs/SELF_HOST.md` §2 as
  required behind any LB / ingress / Cloud Run service.
- `JAMSESH_*_FILE` variants for every secret-bearing env var (DB DSN,
  OAuth client secret, future SMTP creds, etc.). Reads the file at the
  given path on startup; lets operators mount Secret Manager / k8s
  secrets / Docker Swarm secrets without env injection.
- Migration runner wrapped in a Postgres advisory lock so concurrent
  pod starts during a rolling deploy don't race. SQLite path unchanged
  (single-writer already serializes).
- Graceful shutdown: handle `SIGTERM`, stop accepting new connections,
  drain in-flight requests (especially long-running git pushes) within
  a configurable grace window (default 30s), then exit. Hooks into the
  existing `automerger.Worker.Stop` pattern.
- Postgres connection pool config knobs: `JAMSESH_DB_MAX_OPEN_CONNS`,
  `_MAX_IDLE_CONNS`, `_CONN_MAX_LIFETIME`. Cloud SQL / RDS small tiers
  cap total connections aggressively; the current single-DSN config
  has no way to tune.
- `docs/SELF_HOST.md` updates documenting all of the above (Cloud Run /
  Fly / Railway / k8s deploy recipes as appendices).

Out:
- Routing service, lease/fencing, object-storage sync, hydration. Those
  are the four other features in this epic.
- Tracing / OpenTelemetry support (worth doing later; not blocking).
- Log shipping integrations (operators wire their own; structured JSON
  logs already exist).

## Design decisions

Inherited from epic (simple deploy must not regress; clustered mode
opt-in via `JAMSESH_DEPLOY_MODE`; leverage existing `JAMSESH_PORTAL_URL`
not invent a new var). Resolved during feature-design pass under
autopilot:

- **`/metrics` library**: `github.com/prometheus/client_golang` — most
  ergonomic, dominant standard, well-tested. Hand-rolled OpenMetrics
  would save one dep at the cost of every collector being written by
  hand.
- **`/readyz` failure model**: fail-closed binary status (200 ready,
  503 not ready) with structured JSON body listing failed checks.
  Matches k8s readiness-probe and Cloud Run startup-probe contracts.
- **`/readyz` probe set**: DB ping + storage root `os.Stat` only.
  Event log, auto-merger, WS gateway are eventually-consistent
  subsystems and shouldn't gate request acceptance. Smaller probe
  set is harder to misuse.
- **`_FILE` precedence**: `JAMSESH_<VAR>_FILE` takes precedence over
  `JAMSESH_<VAR>` when both are set. File-based is the explicit /
  cloud-native form; operators with both are mid-migration and the
  durable source should win. Document this in SELF_HOST.
- **Migration advisory lock key**: constant `pg_advisory_lock(8675309)`
  (one global lock per portal database — there's no per-tenant
  migration). Memorable, won't collide with future per-session
  advisory-lock keys (which will use `hashtext($session_id)` from
  the lease-fencing feature).
- **Graceful shutdown grace default**: 30s (matches k8s
  `terminationGracePeriodSeconds`). Configurable via
  `JAMSESH_SHUTDOWN_GRACE_S`. Current code uses 25s in `server.Run`
  + 10s in `mergerWorker.Stop` sequentially — total ~35s worst case,
  doesn't fit a single grace window. Refactor to share one grace
  budget across all drain steps.
- **PG pool defaults**: `MaxOpenConns=25, MaxIdleConns=5,
  ConnMaxLifetime=30m`. pgxpool's `MaxConns=4` default is too low for
  any non-trivial deployment; 25 fits comfortably under Cloud SQL
  micro/small tier connection caps (25–100). SQLite pool config knobs
  are accepted but no-op (driver effectively single-writer).
- **`_FILE` helper location**: `internal/portal/config/secrets.go`
  with `readEnvOrFile(name string) string` helper. Reusable for any
  future secret var. Failures (`_FILE` set but unreadable) bubble up
  as Load errors — fail-fast at startup, not silent fallback.
- **Story decomposition**: 6 child stories. Five parallelizable
  implementation chunks + one docs-final story dependent on the
  others. Lets `implement-orchestrator` fan-out to ≤3 sub-agents per
  wave.

## Architectural choice

**Selected: small focused packages where new conceptual surface exists;
inline edits where the surface is purely additive.**

Considered:
- *Option A — single `internal/portal/operational/` package* bundling
  probes + metrics + secrets helpers. Less boilerplate; mixes
  unrelated concerns (a probe failure has nothing to do with metric
  emission); harder to test in isolation.
- *Option B — pure inline edits into existing packages*. Smallest
  diff; but `/readyz` health-checks deserve their own surface for
  future composability (router-layer probe, lease-layer probe, etc.)
  and metrics needs a registry that ought to live somewhere.
- **Option C — focused packages where surface is new; inline where
  it's additive**:
  - New: `internal/portal/probes/` (readyz check composition).
  - New: `internal/portal/metrics/` (Prometheus registry + portal
    counters).
  - New: `internal/portal/config/secrets.go` (read-env-or-file
    helper).
  - Inline: `internal/portal/config/config.go` (new env vars + pool
    config).
  - Inline: `internal/portal/router/router.go` (mount `/readyz`
    and `/metrics`).
  - Inline: `internal/portal/server/server.go` (configurable grace
    budget).
  - Inline: `internal/db/migrate.go` (advisory lock around postgres
    migration; sqlite path unchanged).
  - Inline: `internal/db/connect.go` (apply pool config to pgxpool
    + sqlite `*sql.DB`).
  - Inline: `cmd/portal/main.go` (drain ordering refactor).

Selected: **C**. Matches the existing codebase pattern — small focused
packages (`tokens`, `events`, `storage`, etc.) with clear single
responsibility. Inline where the change is additive within an
existing package.

## Implementation Units

### Unit 1: Readiness probe

**Files**:
- new: `internal/portal/probes/probes.go`
- new: `internal/portal/probes/probes_test.go`
- edit: `internal/portal/router/router.go` (mount `/readyz`; add
  optional `ReadyzCheck` field to `Deps`)
- edit: `cmd/portal/main.go` (wire DB ping + storage stat probes)

**Story**: `epic-cloud-native-deploy-operational-polish-readyz`

```go
// internal/portal/probes/probes.go
package probes

import (
    "context"
    "encoding/json"
    "net/http"
    "time"
)

// Check is a named probe. Name appears in the JSON response body.
// Fn returns nil for healthy, error for unhealthy. Each check runs
// with a 2-second timeout; slow probes report as "timeout".
type Check struct {
    Name string
    Fn   func(ctx context.Context) error
}

// Handler returns an http.Handler that runs every check in parallel
// and responds 200 (all ok) or 503 (any failed) with a JSON body:
//   {"status": "ready" | "not_ready",
//    "checks": [{"name": "...", "ok": true|false, "error": "..."}]}
func Handler(checks []Check) http.Handler { /* ... */ }
```

**Implementation Notes**:
- Each check runs with `context.WithTimeout(2*time.Second)`. A check
  that doesn't return is treated as failed with `error: "timeout"`.
- Checks run in parallel via goroutines + sync.WaitGroup; total
  response time is `max(check_duration)`.
- The response body is JSON regardless of status — operators scrape
  the body to diagnose. The status code is the binary gate.

**Acceptance Criteria**:
- [ ] `GET /readyz` returns 200 with `status: "ready"` when all
  checks pass.
- [ ] `GET /readyz` returns 503 with `status: "not_ready"` and a
  per-check breakdown when any check fails.
- [ ] A check exceeding 2s reports `error: "timeout"`.
- [ ] Parallel execution: 3 checks each taking 1s total ≤ 1.5s.
- [ ] `/healthz` continues to work unchanged.

### Unit 2: Prometheus metrics

**Files**:
- new: `internal/portal/metrics/metrics.go` (registry + collectors)
- new: `internal/portal/metrics/metrics_test.go`
- edit: `internal/portal/router/router.go` (mount `/metrics`)
- edit: `internal/portal/logging/access.go` (emit request-count +
  duration histogram via metrics)
- edit: `cmd/portal/main.go` (initialize registry; pass to router
  and to postreceive emitter for `commit.arrived` counter)
- edit: `go.mod` (add `github.com/prometheus/client_golang`)

**Story**: `epic-cloud-native-deploy-operational-polish-metrics`

```go
// internal/portal/metrics/metrics.go
package metrics

import (
    "net/http"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds the portal's Prometheus registry plus typed handles
// for every metric we emit. Concrete handles avoid string-keyed lookup
// at hot-path emit sites.
type Registry struct {
    HTTPRequestsTotal    *prometheus.CounterVec   // labels: method, route, status
    HTTPRequestDuration  *prometheus.HistogramVec // labels: method, route
    GitPushesTotal       *prometheus.CounterVec   // labels: result (ok|rejected)
    AutoMergerOutcomes   *prometheus.CounterVec   // labels: outcome (succeeded|conflict)
    EventLogEmitTotal    prometheus.Counter
    reg                  *prometheus.Registry
}

// New returns a Registry populated with go-runtime collectors +
// portal-specific metrics. Caller is responsible for adding the
// Handler to the router.
func New() *Registry { /* ... */ }

// Handler returns the HTTP handler that exposes the registry in the
// Prometheus exposition format at /metrics.
func (r *Registry) Handler() http.Handler {
    return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
```

**Implementation Notes**:
- Standard go-runtime collectors (`collectors.NewGoCollector()`) and
  process collector (`collectors.NewProcessCollector(opts{})`) are
  registered automatically.
- Counters use unbounded label cardinality risk; bind route labels
  to chi's `chi.RouteContext(r.Context()).RoutePattern()` (not the
  raw URL) — avoids `/api/sessions/abc123` exploding cardinality.
- The `/metrics` endpoint is unauthenticated. Operators behind LB
  should restrict via network policy; document in SELF_HOST.

**Acceptance Criteria**:
- [ ] `GET /metrics` returns Prometheus text format.
- [ ] Standard go-runtime metrics (`go_goroutines`,
  `go_memstats_*`, `process_cpu_seconds_total`) present.
- [ ] `http_requests_total{method, route, status}` increments on
  every request.
- [ ] `http_request_duration_seconds{method, route}` histogram has
  reasonable buckets (default: 5ms–10s).
- [ ] Routes are recorded as chi route patterns
  (`/api/orgs/{orgID}/sessions/{sessionID}`), not raw URLs.
- [ ] Endpoint is not authenticated.

### Unit 3: `_FILE` secret variants

**Files**:
- new: `internal/portal/config/secrets.go`
- new: `internal/portal/config/secrets_test.go`
- edit: `internal/portal/config/config.go` (every env-overlay site
  for a secret-bearing var calls `readEnvOrFile`)
- edit: doc comment at top of `config.go` (list new `_FILE` variants)

**Story**: `epic-cloud-native-deploy-operational-polish-secrets-from-file`

```go
// internal/portal/config/secrets.go
package config

import (
    "fmt"
    "os"
    "strings"
)

// readEnvOrFile returns the value for env var `name`, preferring
// the contents of the file named by `name + "_FILE"` when that var
// is set. Trailing whitespace (including a trailing newline) is
// trimmed. A `_FILE` variable pointing at an unreadable path is
// fail-fast: returns the read error.
//
// Returns ("", nil) when neither variable is set.
func readEnvOrFile(name string) (string, error) {
    if path := os.Getenv(name + "_FILE"); path != "" {
        b, err := os.ReadFile(path)
        if err != nil {
            return "", fmt.Errorf("config: read %s_FILE (%s): %w", name, path, err)
        }
        return strings.TrimRight(string(b), " \t\r\n"), nil
    }
    return os.Getenv(name), nil
}
```

Env vars gaining `_FILE` variants:
- `JAMSESH_DB_DSN_FILE`
- `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE`
- `JAMSESH_EMAIL_SMTP_PASS_FILE`
- `JAMSESH_EMAIL_SENDGRID_API_KEY_FILE`
- `JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN_FILE`
- `JAMSESH_EMAIL_RESEND_API_KEY_FILE`

**Implementation Notes**:
- The existing `applyEnv` and helpers (`applyEmailEnv`, `applyOAuthEnv`)
  call `os.Getenv` directly. Refactor each secret-bearing line to call
  `readEnvOrFile`. Non-secret env vars (bind, URLs, log level, etc.)
  keep using `os.Getenv` directly — no value in mounting them as
  files.
- `readEnvOrFile` returns `(string, error)` so the caller can fail
  Load. The current `applyEnv` signature is `func(*Config)` with no
  error return; change to `func(*Config) error` and propagate up
  through `Load`. This is a small breaking API change inside the
  package but well-contained.

**Acceptance Criteria**:
- [ ] `_FILE` variant set → reads file, trims trailing whitespace.
- [ ] `_FILE` variant set to unreadable path → `Load` returns error.
- [ ] Both `_FILE` and plain var set → `_FILE` wins.
- [ ] Neither set → empty string, no error.
- [ ] Listed secret env vars in the package doc comment.
- [ ] `applyEnv` returns error; `Load` propagates.

### Unit 4: DB pool config + Postgres migration lock

**Files**:
- edit: `internal/portal/config/config.go` (new `DBConfig` struct:
  `MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime`)
- edit: `internal/db/connect.go` (apply pool config; wrap postgres
  migration in advisory lock)
- edit: `internal/db/migrate.go` (accept optional advisory-lock
  acquire function)
- new: `internal/db/connect_test.go` (pool config wiring;
  migration-lock concurrency test against PG container)

**Story**: `epic-cloud-native-deploy-operational-polish-db-pool-and-lock`

```go
// internal/portal/config/config.go additions
type Config struct {
    // ... existing fields ...
    DB DBConfig `yaml:"db"`
}

type DBConfig struct {
    // MaxOpenConns caps the connection pool size. Default 25.
    // SQLite ignores this (effectively single-writer).
    MaxOpenConns int `yaml:"max_open_conns"`
    // MaxIdleConns caps idle connections in the pool. Default 5.
    MaxIdleConns int `yaml:"max_idle_conns"`
    // ConnMaxLifetime is the maximum time a connection may be reused.
    // Default 30m. Set to 0 for no limit.
    ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}
```

Env vars:
- `JAMSESH_DB_MAX_OPEN_CONNS` (int)
- `JAMSESH_DB_MAX_IDLE_CONNS` (int)
- `JAMSESH_DB_CONN_MAX_LIFETIME` (Go duration: "30m", "1h", etc.)

```go
// internal/db/connect.go: postgres branch additions
cfg.MaxConns         = int32(dbcfg.MaxOpenConns)
cfg.MinConns         = int32(dbcfg.MaxIdleConns)
cfg.MaxConnLifetime  = dbcfg.ConnMaxLifetime

// migration wrapping
mdb := stdlib.OpenDBFromPool(pool)
if err := withMigrationLock(ctx, mdb, func() error {
    return MigrateUp(ctx, mdb, "postgres")
}); err != nil {
    mdb.Close(); pool.Close()
    return nil, fmt.Errorf("migrate: %w", err)
}
mdb.Close()
```

```go
// internal/db/migrate.go: lock helper
// jamseshMigrationLockKey is a constant int64 used as the
// pg_advisory_lock key for all portal migration runs. Chosen as
// a memorable arbitrary number; will not collide with future
// per-session advisory locks which use hashtext(session_id).
const jamseshMigrationLockKey int64 = 8675309

// withMigrationLock acquires pg_advisory_lock(jamseshMigrationLockKey)
// before invoking fn, releases on completion. Blocks if another portal
// instance is mid-migration. SQLite callers don't use this helper.
func withMigrationLock(ctx context.Context, db *sql.DB, fn func() error) error {
    if _, err := db.ExecContext(ctx,
        "SELECT pg_advisory_lock($1)", jamseshMigrationLockKey,
    ); err != nil {
        return fmt.Errorf("acquire migration lock: %w", err)
    }
    defer db.ExecContext(context.Background(),
        "SELECT pg_advisory_unlock($1)", jamseshMigrationLockKey)
    return fn()
}
```

**Implementation Notes**:
- SQLite pool config is silently accepted but has no operational
  effect (modernc.org/sqlite is single-writer; setting MaxOpenConns
  > 1 doesn't change concurrency semantics). Document this.
- `pg_advisory_lock` is a session-scoped lock. The migration runs on
  the temporary `*sql.DB` opened from pgxpool; the lock is released
  via the deferred call before the connection closes. If the process
  dies mid-migration the lock auto-releases (PG session ended) and
  the next pod tries again — safe.
- The lock acquire is unconditional (`pg_advisory_lock`, not
  `pg_try_advisory_lock`). Pods that start mid-migration block until
  the leader finishes. This is correct: every pod runs the same
  migrations; the second pod's MigrateUp will be a no-op (goose
  idempotency).

**Acceptance Criteria**:
- [ ] `MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime` configurable
  via YAML and env.
- [ ] Postgres pool reflects configured values (`pool.Config()`
  inspection in test).
- [ ] SQLite open succeeds even with pool values set.
- [ ] Postgres migration acquires advisory lock before running.
- [ ] Two concurrent `db.Open` calls against the same PG database
  serialize on the lock; both succeed; migrations run once
  effectively.
- [ ] Lock is released after migration completes.

### Unit 5: Configurable graceful shutdown

**Files**:
- edit: `internal/portal/config/config.go` (new
  `ShutdownGraceSeconds int` field; env
  `JAMSESH_SHUTDOWN_GRACE_S`)
- edit: `internal/portal/server/server.go` (use
  `cfg.ShutdownGraceSeconds` instead of hardcoded 25s)
- edit: `cmd/portal/main.go` (refactor drain ordering to share
  grace budget; sequence: stop accepting → drain HTTP →
  mergerWorker.Stop → wsGateway.Stop, all inside one budget)

**Story**: `epic-cloud-native-deploy-operational-polish-graceful-shutdown`

```go
// internal/portal/config/config.go additions
type Config struct {
    // ... existing fields ...
    ShutdownGraceSeconds int `yaml:"shutdown_grace_s"`
}

// defaults():
//   ShutdownGraceSeconds: 30,
```

```go
// internal/portal/server/server.go change
case <-ctx.Done():
    grace := time.Duration(cfg.ShutdownGraceSeconds) * time.Second
    slog.InfoContext(context.Background(), "portal shutting down",
        "drain_budget_s", cfg.ShutdownGraceSeconds)
    shutCtx, cancel := context.WithTimeout(context.Background(), grace)
    defer cancel()
    return srv.Shutdown(shutCtx)
```

```go
// cmd/portal/main.go: shared drain budget
// Build the drain context once SIGTERM arrives; share it across
// HTTP shutdown, mergerWorker.Stop, and wsGateway.Stop so the
// total wall-clock for shutdown is bounded by the grace budget,
// not multiplied across subsystems.
//
// Order: HTTP server stops accepting first (server.Run handles
// this internally), then we wait on in-flight handlers via the
// same drain context; auto-merger and WS gateway drain in parallel
// once HTTP is quiet.
```

**Implementation Notes**:
- The current code does sequential drain: server.Run (25s) → then
  `mergerWorker.Stop(10s)` → `wsGateway.Stop()`. Worst case ~35s.
  Refactor to a shared `drainCtx` derived from the grace budget.
- The `server.Run` exit point is when HTTP shutdown completes; the
  remaining grace is what's left of `cfg.ShutdownGraceSeconds -
  elapsed`. Pass remaining time to `mergerWorker.Stop`.
- WS gateway stop is fast (just unsubscribes); doesn't need to share
  the budget meaningfully.
- If grace expires mid-drain, the deferred shutdown still cleans up
  what it can; HTTP connections that were in-flight get a TCP RST.
  Log a warning at the end documenting any timeouts.

**Acceptance Criteria**:
- [ ] `JAMSESH_SHUTDOWN_GRACE_S` env / `shutdown_grace_s` YAML
  configurable; default 30.
- [ ] SIGTERM causes server to stop accepting new connections.
- [ ] In-flight HTTP requests complete within the grace window.
- [ ] Auto-merger drains within the same budget.
- [ ] WS gateway stop completes.
- [ ] Test injects SIGTERM, asserts in-flight request (slow handler)
  completes before exit.
- [ ] Test with grace=1s asserts the slow handler is cut off and the
  process exits within 2s.

### Unit 6: SELF_HOST.md update + SPEC.md deployment-shape touch

**Files**:
- edit: `docs/SELF_HOST.md` (§2 Configuration table: add all new
  env vars + `_FILE` convention paragraph; §8 Monitoring: replace
  "future" note with `/metrics` reference; §9 Upgrade procedure:
  note configurable grace window; new §13 Cloud deploy recipes
  for Cloud Run, Fly, Railway, k8s with PVC)
- edit: `docs/SPEC.md` (§"Deployment shape": list the new env vars
  alongside existing ones — keep tone "present truth")

**Story**: `epic-cloud-native-deploy-operational-polish-docs`

**Implementation Notes**:
- Cloud deploy recipes should be concrete and runnable, not
  abstract. Per cloud:
  - **Cloud Run**: `gcloud run deploy` command with all flags;
    note `min-instances=1` for keep-alive sessions, mention the
    60-min request timeout cap affecting WebSockets.
  - **Fly**: `fly.toml` snippet with `[[mounts]]` for persistent
    volume, `[deploy].strategy=immediate`, grace_period setting.
  - **Railway**: railway.json/Procfile sketch; reference its
    auto-detect.
  - **k8s with PVC**: Deployment + Service + PVC yaml example;
    readiness probe pointing at `/readyz`;
    `terminationGracePeriodSeconds: 35` (grace + small buffer).
- Each recipe documents which env vars are critical for that
  cloud (`JAMSESH_PORTAL_URL` is universal; `JAMSESH_*_FILE` for
  Secret Manager; etc.).
- Keep SPEC.md edits minimal and present-truth: only add env vars
  that now exist. Don't pre-document clustered-mode features.

**Acceptance Criteria**:
- [ ] SELF_HOST §2 lists every new env var with type / default /
  purpose.
- [ ] SELF_HOST has a `_FILE` convention paragraph explaining
  precedence and use-cases.
- [ ] SELF_HOST §8 documents `/metrics` and `/readyz`.
- [ ] SELF_HOST §9 documents the configurable shutdown grace.
- [ ] SELF_HOST has a new section with at least the four cloud
  deploy recipes.
- [ ] SPEC.md "Deployment shape" reflects the new env vars
  without contradicting the single-binary self-host invariant.

## Implementation Order

Stories 1-5 are parallel-ready (no inter-dependencies). Story 6
depends on all five so the documentation reflects final shape.

1. (parallel) Unit 1, 2, 3, 4, 5
2. Unit 6 (after 1-5 are at `stage: review`)

## Testing

| Unit | Test type | Key assertions |
|---|---|---|
| 1 readyz | `httptest` against `probes.Handler` | 200 on all-ok, 503 on any-fail, parallel timing, JSON envelope |
| 2 metrics | `httptest` against `metrics.Registry.Handler()` | go-runtime metrics present; counter increments on simulated request; route labels match chi patterns |
| 3 secrets | unit tests on `readEnvOrFile` | env-only, file-only, both (file wins), neither, unreadable file |
| 4 db pool + lock | unit (pool wiring) + integration (PG container, concurrent migrate) | pool config reflected; lock serializes two pods |
| 5 graceful shutdown | integration test with SIGTERM injection | in-flight request completes; short-grace cuts off |
| 6 docs | manual review during PR | every new env var present in SELF_HOST table; SPEC.md still passes the foundation-doc roll-forward principle |

## Risks

- **Prometheus client lib adds a dependency.** Mature, widely-used,
  but pulls in transitive deps. Mitigation: pin a known-good version;
  treat upgrades as deliberate.
- **`/metrics` cardinality from un-bounded route labels.** Using chi's
  RoutePattern (not raw URL) caps cardinality. Misconfiguration could
  still explode the metric series. Mitigation: test that route labels
  are patterns; document the constraint.
- **Migration lock could deadlock if a future code path reuses
  `8675309`.** Mitigation: comment in `migrate.go` declares the
  constant; lease-fencing feature will use `hashtext(session_id)`
  which can't collide with the fixed key.
- **Shared shutdown grace budget can starve auto-merger** if a slow
  HTTP request consumes most of the window. Mitigation: log per-step
  elapsed at shutdown; operators tune via `JAMSESH_SHUTDOWN_GRACE_S`.
- **`_FILE` failing at startup is loud, not silent.** That's the
  intended behavior — silent fallback to env would mask credential
  mismount in production. Risk is operator surprise; documented.

## Foundation-doc impact

- `docs/SPEC.md` — Deployment-shape section grows by the new env
  vars when this feature reaches `stage: done`.
- `docs/SELF_HOST.md` — substantial updates per Unit 6.
