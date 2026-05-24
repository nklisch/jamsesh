---
id: feature-epic-ephemeral-playground-reserved-org
kind: feature
stage: done
tags: [portal]
parent: epic-ephemeral-playground
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Reserved playground org provisioning + config

## Brief

Adds the config knobs that gate playground availability and the
idempotent startup hook that provisions the reserved system-owned
`playground` org. When `JAMSESH_PLAYGROUND_ENABLED=true`, `cmd/portal/main.go`
seeds the org row on every boot (idempotent via `slug = 'playground'`
uniqueness); when false, no provisioning runs and any playground REST
route returns `503` to signal the feature is disabled for this
deployment.

The reserved org gets an `org_protected: true` boolean column on `orgs`
set at provisioning time. Any handler that mutates or deletes orgs must
check this flag and reject with `409 org.protected` — this is the data-
layer guard against the playground org being accidentally destroyed by
an unrelated future feature (defense in depth, not handler-level only).

Config knobs introduced (per the strategic-decisions section of the
parent epic):
- `JAMSESH_PLAYGROUND_ENABLED` (bool, default `false`)
- `JAMSESH_PLAYGROUND_IDLE_TIMEOUT` (duration, default `30m`)
- `JAMSESH_PLAYGROUND_HARD_CAP` (duration, default `24h`)
- Abuse-cap env vars are listed and reserved here but consumed in
  `session-lifecycle`: `JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR` (default
  `3`), `JAMSESH_PLAYGROUND_MAX_PARTICIPANTS` (default `5`),
  `JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES` (default `50 << 20`, 50 MiB).

This feature is config + startup substrate only. It does NOT add the
playground REST routes, the destruction worker, or anything user-
visible. Those live in `session-lifecycle`.

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 1 foundation** — no dependencies; required by
  `session-lifecycle` (wave 2) for the playground org row to exist
  before sessions can be created inside it.

## Foundation references
- `docs/SPEC.md` § Hard constraints + § Deployment shape — the
  multi-tenant invariant the reserved-org pattern preserves; the env-var
  list this feature extends
- `docs/ARCHITECTURE.md` § Data layer (multi-tenancy) — the
  "Reserved orgs" paragraph added at scope time describes this
  feature's runtime contract
- `docs/SELF_HOST.md` — env-var reference table roll-forward is owned
  by this feature's design pass

## Mockups
No UI surface — config + startup substrate. The parent epic's flow
mocks cover everything user-visible.

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

- **Reserved org slug**: hardcoded `playground`. Single canonical value
  across every deployment. Docs, support material, observability
  dashboards, and `pre-receive` checks can hard-reference `org:playground`
  without env-var lookup. If an operator has a pre-existing real org
  named `playground` and tries to enable the feature, the startup
  provisioning hook detects the slug collision (the existing row has
  no `org_protected: true`), logs a clear conflict error, and refuses
  to enable playground until the operator renames their org. No silent
  upgrade-then-merge surprise.

- **Disable-flip behavior** (`JAMSESH_PLAYGROUND_ENABLED` true → false):
  reject new creates immediately; let active sessions age out
  naturally. `POST /api/playground/sessions` returns
  `503 playground.disabled`; the join endpoint also returns 503 for new
  joiners. Existing in-flight sessions keep running through their
  normal idle / hard-cap lifecycles — the destruction sweep continues
  to fire even when the create endpoint is off. Within 24h (hard cap),
  the deployment is naturally playground-free. Lowest surprise to
  in-flight participants. Operators who need an immediate shutdown
  can still trigger a manual destruction sweep via ops tooling
  (out-of-scope for this feature).

- **`org_protected` scope**: block delete + rename only. The flag
  rejects `DELETE /api/orgs/{id}` and `PATCH /api/orgs/{id}` mutations
  to name/slug. Member-add operations against the playground org are
  still allowed (preserves flexibility for a future ops/observability
  use case where a human is added to inspect the playground org's
  sessions). Other writes (session create, member-leave, etc.) go
  through their own auth + tenancy checks; `org_protected` is
  specifically about the org row's identity stability.

- **Provisioning on upgrade**: only when the operator opts in. Existing
  deployments stay byte-identical after upgrading to the version that
  ships playground. No `playground` org row appears until the operator
  explicitly sets `JAMSESH_PLAYGROUND_ENABLED=true` and restarts the
  portal. First-opt-in startup seeds the org row idempotently. Loss:
  `org_protected: true` can't be enforced on a pre-existing user-owned
  `playground` org row that survives an upgrade — addressed by the
  startup-conflict check from the first decision above (the conflict
  detection runs every boot, not just at first provisioning).

## Architectural choice

**Goose migration for the schema change + new `playground` package for
the startup-time provisioning hook + Config struct extension for env
vars.** The provisioning hook follows the existing `FindOrProvision`
pattern (in `internal/portal/auth/provision.go`) but is invoked at
startup rather than per-user — a sibling helper at
`internal/portal/playground/provision.go`.

Why this shape:
- **Config struct extension** keeps env var documentation and YAML
  binding in one place per the existing Config pattern
- **Schema migration** adds `orgs.org_protected` cleanly via ALTER
  TABLE in both dialects (no rebuild needed — this is an additive
  column with a default)
- **Dedicated `playground` package** is the natural home for the
  startup hook AND for the abuse-cap consumers in the
  `session-lifecycle` sibling feature (which will land later units in
  the same package). Setting up the package home here keeps that
  feature's surface area predictable

Why over alternatives:
- **Inline provisioning in `cmd/portal/main.go`**: would bloat the
  main entrypoint and couple startup logic to argument parsing.
  Extracted helpers are testable; main is not.
- **Provision via REST endpoint called at startup**: heavyweight,
  introduces a self-call dance, and the org-provisioning shouldn't be
  exposed as a public endpoint at all.

## Implementation units

Single tight-cohesion feature — **no child stories spawned**. The
design body IS the implementation guide. 5 sequential units; one
implementing agent walks the sequence.

### Unit 1: Config struct extension
**File**: `internal/portal/config/config.go`

Add the following fields to the `Config` struct, following the
existing inline-documentation pattern:

```go
// PlaygroundEnabled gates the entire ephemeral-playground subsystem.
// false (default) — playground REST routes return 503; no reserved
// `playground` org is provisioned at startup. true — startup
// provisions the reserved org idempotently and the routes accept
// traffic, subject to the abuse caps below.
// Env: JAMSESH_PLAYGROUND_ENABLED
PlaygroundEnabled bool `yaml:"playground_enabled"`

// PlaygroundIdleTimeoutS is the idle-timeout window for playground
// sessions (seconds). A session whose `last_substantive_activity_at`
// is older than this is destroyed by the next sweep. Default: 1800 (30m).
// Env: JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S
PlaygroundIdleTimeoutS int `yaml:"playground_idle_timeout_s"`

// PlaygroundHardCapS is the wall-clock cap on playground session
// lifetime (seconds, measured from session creation). Whichever fires
// first between idle and hard cap, the session ends. Default: 86400 (24h).
// Env: JAMSESH_PLAYGROUND_HARD_CAP_S
PlaygroundHardCapS int `yaml:"playground_hard_cap_s"`

// PlaygroundCreatePerIPHour caps anonymous session creation per IP
// per hour. Reserved here; consumed by the session-lifecycle feature's
// rate-limiting middleware. Default: 3.
// Env: JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR
PlaygroundCreatePerIPHour int `yaml:"playground_create_per_ip_hour"`

// PlaygroundMaxParticipants caps concurrent participants per
// playground session. Reserved here; consumed by session-lifecycle.
// Default: 5.
// Env: JAMSESH_PLAYGROUND_MAX_PARTICIPANTS
PlaygroundMaxParticipants int `yaml:"playground_max_participants"`

// PlaygroundMaxContentBytes is the per-session accumulated content
// cap (bytes). pre-receive rejects pushes that would exceed it.
// Reserved here; consumed by session-lifecycle. Default: 52428800 (50 MiB).
// Env: JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES
PlaygroundMaxContentBytes int64 `yaml:"playground_max_content_bytes"`

// PlaygroundDestructionSweepIntervalS is how often the destruction
// worker walks active playground sessions to apply idle/hard-cap
// expiry. Default: 60.
// Env: JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S
PlaygroundDestructionSweepIntervalS int `yaml:"playground_destruction_sweep_interval_s"`
```

Update the env-var-to-Config-field loader (look for the existing
`applyEnvOverrides` or equivalent function in `config.go`; add
matching cases for each new env var). Apply defaults if env var is
unset and YAML doesn't specify.

**Acceptance criteria**:
- [ ] All 7 new fields added with inline doc comments matching the
      existing style
- [ ] Env var overrides applied correctly (test via the existing
      `TestApplyEnvOverrides` pattern)
- [ ] Defaults applied when neither env nor YAML sets the field
      (`PlaygroundEnabled=false`, `PlaygroundIdleTimeoutS=1800`, etc.)
- [ ] Boolean parsing for `JAMSESH_PLAYGROUND_ENABLED` accepts the
      same set of values as other bool env vars (likely `"true"/"1"/"yes"`
      per the existing pattern; verify)

---

### Unit 2: Schema migration (orgs.org_protected column)
**File**: `internal/db/migrations/sqlite/NNNN_org_protected.sql`
       + `internal/db/migrations/postgres/NNNN_org_protected.sql`

Both dialects support `ALTER TABLE ADD COLUMN` with a default — no
rebuild needed. The same migration file shape:

**SQLite**:
```sql
-- +goose Up
ALTER TABLE orgs ADD COLUMN org_protected INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite doesn't support DROP COLUMN before 3.35.0; the project's
-- minimum SQLite version is 3.x.y (check go.mod for the SQLite driver
-- version and confirm 3.35+ is the floor). If yes, use:
-- ALTER TABLE orgs DROP COLUMN org_protected;
-- If not, the Down migration is a no-op with a comment explaining why.
```

**Postgres**:
```sql
-- +goose Up
ALTER TABLE orgs ADD COLUMN org_protected BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE orgs DROP COLUMN org_protected;
```

**Also update** the source-of-truth schema files (`db/schema/sqlite.sql`
and `db/schema/postgres.sql`) — add `org_protected` to the `orgs` table
definition. sqlc reads from these.

**Acceptance criteria**:
- [ ] Migration applies on fresh + existing DB in both dialects
- [ ] Existing org rows pick up `org_protected = 0/FALSE` automatically
      (verified by snapshot-then-migrate test)
- [ ] sqlc regenerates without errors; generated `Org` struct exposes
      `OrgProtected bool` field
- [ ] Existing `CreateOrg` query doesn't break — `org_protected`
      defaults to false for new org rows (no need to mention the column
      in the existing `INSERT` statement)

---

### Unit 3: sqlc query for protected-marker provisioning
**File**: `db/queries/sqlite/orgs.sql` + postgres mirror

```sql
-- name: GetOrgBySlug :one
SELECT * FROM orgs WHERE slug = ? LIMIT 1;

-- name: CreateProtectedOrg :one
-- Inserts an org row with org_protected=true. Used at startup by the
-- playground provisioning hook for the reserved `playground` org.
-- Slug uniqueness is enforced by the existing UNIQUE constraint on orgs.slug.
INSERT INTO orgs (id, name, slug, session_invite_policy, created_at, org_protected)
VALUES (?, ?, ?, 'open', ?, 1)
RETURNING *;
```

(Postgres uses `$1, $2, ...` placeholders and `TRUE`.)

The protected-org provisioning uses `session_invite_policy='open'`
because playground sessions are open-join by design (no `org_members`
gating on the playground org).

**Acceptance criteria**:
- [ ] sqlc generates `GetOrgBySlug` and `CreateProtectedOrg` methods
- [ ] `GetOrgBySlug` returns the matching org or `pgx.ErrNoRows` /
      `sql.ErrNoRows` for missing
- [ ] `CreateProtectedOrg` sets `org_protected = true`,
      `session_invite_policy = 'open'`
- [ ] Round-trips correctly via `stores(t)` test harness

---

### Unit 4: Playground provisioning package + startup hook
**Files**:
- `internal/portal/playground/provision.go` (new package)
- `internal/portal/playground/provision_test.go`
- `cmd/portal/main.go` (call site)

```go
// internal/portal/playground/provision.go
package playground

import (
    "context"
    "errors"
    "fmt"
    "log/slog"

    "<module>/internal/db/store"
)

// ReservedOrgSlug is the hardcoded slug for the system-owned playground
// org. Per the parent epic's strategic decision, this is NOT configurable.
const ReservedOrgSlug = "playground"

// ReservedOrgID is the deterministic ID for the playground org.
// Using a deterministic ID lets observability dashboards hard-reference
// it without env-var lookup.
const ReservedOrgID = "org_playground"

// ReservedOrgName is the human-readable name.
const ReservedOrgName = "Playground"

// ProvisionReservedOrg ensures the reserved `playground` org row exists
// when playground is enabled. Idempotent: safe to call on every boot.
//
// Behavior:
//   - If no org with slug "playground" exists: creates a protected org row.
//   - If a protected org with slug "playground" exists: no-op, returns nil.
//   - If an UNPROTECTED org with slug "playground" exists (operator had
//     a real org by that name): refuses to start. Returns ErrReservedSlugConflict
//     so cmd/portal/main.go can log a clear actionable error and exit.
func ProvisionReservedOrg(ctx context.Context, s store.Store, now time.Time, logger *slog.Logger) error {
    existing, err := s.GetOrgBySlug(ctx, ReservedOrgSlug)
    if err != nil && !errors.Is(err, store.ErrNoRows) {
        return fmt.Errorf("lookup existing %s org: %w", ReservedOrgSlug, err)
    }

    if err == nil {
        // Org exists. Check protection flag.
        if !existing.OrgProtected {
            return fmt.Errorf("%w: an unprotected org with slug %q exists (id=%s); rename it before enabling playground",
                ErrReservedSlugConflict, ReservedOrgSlug, existing.ID)
        }
        // Already provisioned; idempotent no-op.
        logger.Info("playground org already provisioned", "org_id", existing.ID)
        return nil
    }

    // No org with that slug exists; provision it.
    org, err := s.CreateProtectedOrg(ctx, store.CreateProtectedOrgParams{
        ID:        ReservedOrgID,
        Name:      ReservedOrgName,
        Slug:      ReservedOrgSlug,
        CreatedAt: now,
    })
    if err != nil {
        return fmt.Errorf("create %s org: %w", ReservedOrgSlug, err)
    }
    logger.Info("playground org provisioned", "org_id", org.ID)
    return nil
}

// ErrReservedSlugConflict signals that an unprotected org claims the
// reserved playground slug. The portal refuses to start in this state.
var ErrReservedSlugConflict = errors.New("reserved slug conflict")
```

**`cmd/portal/main.go` call site** (after migrations, before HTTP serve):

```go
if cfg.PlaygroundEnabled {
    if err := playground.ProvisionReservedOrg(ctx, s, time.Now().UTC(), logger); err != nil {
        // Conflict is fatal: refuse to start until operator resolves.
        if errors.Is(err, playground.ErrReservedSlugConflict) {
            logger.Error("playground enabled but reserved slug is taken — refusing to start",
                "err", err, "remediation", "rename the existing 'playground' org or set JAMSESH_PLAYGROUND_ENABLED=false")
            os.Exit(1)
        }
        // Other errors (transient DB failure): retry on next start
        logger.Error("playground org provisioning failed", "err", err)
        os.Exit(1)
    }
}
```

**Implementation notes**:
- `store.ErrNoRows` may not exist as a typed error in the store package
  today; if not, use `errors.Is(err, sql.ErrNoRows)` (or `pgx.ErrNoRows`
  for the Postgres path). Confirm during implementation.
- The conflict-detection path fires on EVERY boot, not just first
  provisioning — so if an operator post-creation renames the protected
  playground org to e.g. `playground-OLD` and then someone else creates
  a regular org named `playground`, the next boot catches it.
- The deterministic `org_playground` ID is intentional: it makes the
  reserved-org row identifiable in support tickets and observability
  without a slug lookup. UUID-style randomness adds no value here
  (only one row of this kind ever exists per deployment).

**Acceptance criteria**:
- [ ] First boot with `PlaygroundEnabled=true` and no existing
      `playground` org: creates the row, logs "provisioned"
- [ ] Second boot (no change): no-op, logs "already provisioned",
      no new row inserted
- [ ] Pre-existing unprotected org with slug `playground`: returns
      `ErrReservedSlugConflict`, main exits 1 with a clear error
- [ ] `PlaygroundEnabled=false`: hook isn't called; no `playground`
      org row appears even if the schema column exists
- [ ] Transient DB failure during provisioning: returns wrapped error;
      operator can fix DB and restart
- [ ] `provision_test.go` tests run via `stores(t)` harness covering
      all three branches

---

### Unit 5: `docs/SELF_HOST.md` env-var reference roll-forward
**File**: `docs/SELF_HOST.md`

Add a new section near the existing env-var reference (likely near
"Configuration" or "Environment variables"):

```markdown
## Playground configuration

Ephemeral anonymous playground sessions are an operator-opt-in feature.
The default deployment ships with playground disabled.

| Env var | Default | Effect |
|---|---|---|
| `JAMSESH_PLAYGROUND_ENABLED` | `false` | Master switch. When `true`, startup provisions the reserved `playground` org and the `/api/playground/*` routes serve traffic. When `false`, those routes return 503. |
| `JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S` | `1800` (30m) | Idle window for active playground sessions. A session whose last substantive activity (commit, comment, finalize-attempt) is older than this is destroyed by the destruction sweep. |
| `JAMSESH_PLAYGROUND_HARD_CAP_S` | `86400` (24h) | Wall-clock cap on session lifetime since creation. Whichever of idle / hard-cap fires first ends the session. |
| `JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR` | `3` | Per-IP rate limit on `POST /api/playground/sessions` (session creation). |
| `JAMSESH_PLAYGROUND_MAX_PARTICIPANTS` | `5` | Cap on concurrent participants per playground session. Excess joiners get a friendly "session full" page. |
| `JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES` | `52428800` (50 MiB) | Per-session accumulated push throughput cap. Enforced at `pre-receive`. |
| `JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S` | `60` | How often the destruction worker walks active playground sessions to apply idle and hard-cap expiry. |

### Conflict resolution

If the database already contains an unprotected org with slug
`playground` (e.g. a real org you created before enabling this feature),
the portal will refuse to start with a clear error message. Rename the
existing org (`UPDATE orgs SET slug = 'playground-renamed' WHERE id = '<id>'`)
and restart, or unset `JAMSESH_PLAYGROUND_ENABLED` to disable the
feature.

### Disable behavior

Flipping `JAMSESH_PLAYGROUND_ENABLED` from `true` to `false` rejects
new session creates immediately (503). **Active sessions are not
force-terminated** — they age out naturally through the destruction
sweep (which continues to run regardless of the enabled flag). Within
the configured hard-cap window (default 24h), the deployment is
naturally playground-free.

For an immediate shutdown, the operator can additionally invoke a
manual destruction sweep via the ops tooling (out of scope; documented
when the ops surface lands).
```

**Acceptance criteria**:
- [ ] Section added to SELF_HOST.md with the 7-row env var table
- [ ] Conflict-resolution subsection explains the unprotected-slug
      collision case with a concrete remediation SQL example
- [ ] Disable-behavior subsection explains the natural-age-out semantics
- [ ] Reads cleanly within the surrounding SELF_HOST.md prose; uses
      present-tense / rolling-foundation language (no "this is new in
      v0.4.0" / "previously" / etc.)

---

## Implementation order

Sequential — each step depends on the prior:

1. **Unit 1**: Config struct fields + env loader (no DB or runtime
   dependencies)
2. **Unit 2**: Schema migration + `db/schema/*.sql` update + `sqlc generate`
3. **Unit 3**: sqlc queries (regenerate after schema)
4. **Unit 4**: Provisioning package + `cmd/portal/main.go` wiring
5. **Unit 5**: SELF_HOST.md update (anytime; recommended last to reflect
   actual implemented behavior)

No fan-out — single implementing agent walks the sequence.

## Testing

- `internal/portal/config/config_test.go` (extend) — env-var override
  tests for the 7 new fields; defaults applied correctly
- `internal/portal/playground/provision_test.go` (new) — three-branch
  coverage (no-existing-org / already-provisioned / unprotected-slug-conflict),
  via `stores(t)` harness for both dialects
- `internal/db/store/orgs_test.go` (extend or add) — round-trip
  `CreateProtectedOrg` and `GetOrgBySlug`
- `internal/db/migrate_test.go` (extend) — verify Up + Down (or Up-only
  if SQLite version doesn't support DROP COLUMN) for the new migration
- `cmd/portal/main_test.go` (extend if it exists, otherwise add) — verify
  the conflict-exit path (set `PlaygroundEnabled=true` against a DB with
  an unprotected `playground` org; main exits 1)

## Risks

- **Guard rails are pure defense-in-depth today**: no Delete/RenameOrg
  endpoints exist in the codebase right now. The `org_protected` column
  is enforced nowhere. Any future PR adding such an endpoint must
  consult the column. Mitigation: add a comment in
  `internal/db/store/store.go` (or in `internal/portal/accounts/orgs.go`
  near the existing `UpdateOrgSessionInvitePolicy` handler) noting "any
  handler that mutates or deletes an org MUST check `OrgProtected`
  and return 409 if true." Cheap, durable reminder.

- **SQLite DROP COLUMN compatibility**: the Down migration uses
  `ALTER TABLE ... DROP COLUMN` which requires SQLite 3.35.0+. Check
  the SQLite driver version in `go.mod` (likely `mattn/go-sqlite3` or
  `modernc.org/sqlite`). If the minimum bundled version is below 3.35,
  the Down migration is a no-op with a comment; goose still tracks
  the version so the schema state is consistent. Lost capability:
  rolling back the migration on an old SQLite is impossible; not a
  concern in practice (forward-only schema discipline).

- **Deterministic `org_playground` ID vs `accounts.id` collision**:
  the project's account IDs likely follow `acc_<random>` and org IDs
  follow `org_<random>` patterns per the existing factory functions.
  `org_playground` is intentionally non-random — confirm there's no
  uniqueness constraint or check that would reject the literal string.
  `orgs.id TEXT PRIMARY KEY` accepts any string; should be fine.

- **`session_invite_policy='open'` for the playground org**: makes the
  reserved org's `AcceptSessionInvite` semantics permissive for
  playground sessions. Aligns with the "open-join" nature of playground
  but is worth noting in case future security review questions the
  choice. The protected flag prevents this policy from being changed
  via `UpdateOrgSessionInvitePolicy` (which is an existing handler) —
  but per the locked design decision, `org_protected` only blocks
  delete + rename, not policy changes. So technically an operator could
  flip the playground org's policy to `members_only` via the existing
  PATCH handler, which would break playground (no anonymous user
  satisfies the org-member check). **Inconsistency** — the design says
  "delete + rename only" but the playground org's open policy IS
  load-bearing for the feature. Resolution: log this discovery during
  implementation; either extend the guard to also cover policy changes
  for the playground org specifically (small handler addition), or
  accept the operator-foot-gun and document. Recommended: extend the
  guard. Cost: one extra `if org.OrgProtected { return 409 }` line in
  the `UpdateOrgSessionInvitePolicy` handler.

## Implementation notes

### Units completed

1. **Unit 1 — Config struct**: 7 new fields added to `Config` struct with
   inline doc comments; `applyPlaygroundEnv()` function added to
   `config.go`; `JAMSESH_PLAYGROUND_ENABLED` accepts `"true"`, `"1"`,
   `"yes"` (consistent with the feature spec; note this differs slightly
   from `AuthRateLimitEnabled` which uses `v != "false"` — here we use
   explicit positive matching for the safer default-false bool).

2. **Unit 2 — Schema migration**: `00017_org_protected.sql` added for both
   SQLite and Postgres dialects. SQLite Down migration uses `DROP COLUMN`
   (available since SQLite 3.35.0; `modernc.org/sqlite` bundles 3.49+).
   Schema source-of-truth files (`db/schema/sqlite.sql`,
   `db/schema/postgres.sql`) updated with the new column.
   `ListOrgsForAccount` query (in `org_members.sql` for both dialects)
   also updated to include `org_protected` — necessary because the
   generated row type changed to `Org` (was `ListOrgsForAccountRow`
   before the column existed).

3. **Unit 3 — sqlc queries**: `CreateProtectedOrg` and updated
   `GetOrgBySlug` / `GetOrgByID` added to both dialect query files.
   `sqlc generate` ran cleanly. `OrgProtected` is `int64` in the SQLite
   model (stored as `INTEGER NOT NULL DEFAULT 0`) and `bool` in the
   Postgres model; the store adapters map them both to `bool` in the
   domain `Org` struct (`row.OrgProtected != 0` for SQLite).

4. **Unit 4 — Playground package + startup hook**: `internal/portal/playground/provision.go`
   created; `ProvisionReservedOrg` implements all three branches (no-op,
   create, conflict). `cmd/portal/main.go` updated with the
   `if cfg.PlaygroundEnabled { ... }` block immediately after the DB is
   opened and migrations have run. `internal/portal/playground/provision_test.go`
   covers all three branches via the SQLite dialect harness (Postgres
   skipped unless `JAMSESH_TEST_PG_DSN` is set).
   `internal/portal/handlerauth/handlerauth_test.go`'s `stubStore` updated
   to implement the new `CreateProtectedOrg` method (interface compliance).

5. **Unit 5 — SELF_HOST.md**: Section 15 "Playground configuration" added
   at the end of the document with the 7-row env var table, Conflict
   resolution subsection, and Disable behavior subsection.

### Deviations from design

- The design spec skeleton showed `store.ErrNoRows` as the sentinel to
  check. The actual store uses `store.ErrNotFound` (the canonical error
  returned by all Get* methods). The implementation uses `store.ErrNotFound`.

- `GetOrgBySlug` already existed in both query files (used by other
  features). The column list was updated from explicit column enumeration
  to include `org_protected`; the feature spec's instruction to add it as
  a new query was interpreted as "update the existing query to return the
  new column" rather than adding a duplicate.

- `ListOrgsForAccount` in `org_members.sql` also required updating to add
  `org_protected` to the SELECT list; otherwise the generated Go type
  (`ListOrgsForAccountRow`) would not match `sqlitestore.Org` in the
  adapter's `sqliteOrg()` call. This was not called out in the feature
  design but is a natural consequence of adding the column to the schema.

### Verification status

- `sqlc generate`: clean, no errors
- `go build ./...`: passes
- `go test ./internal/db/... ./internal/portal/playground/... ./internal/portal/config/... ./cmd/portal/...`: all pass
- `go test ./...` (full suite, 54 packages): all pass
- `go vet ./...`: clean

## Review (2026-05-23)

**Verdict**: Approve with comments

**Blockers**: none

**Important**:
- Risk #4 from the design (the `session_invite_policy='open'` foot-gun)
  was neither resolved by extending the guard nor explicitly accepted
  in the implementation notes. Tracked as backlog item
  `extend-org-protected-guard-to-policy-mutations`. Not blocking — the
  PatchOrg handler currently requires `member.Role == "creator"` and
  the playground org has no creator account, so the path is
  unreachable in practice today.

**Nits**:
- `db/schema/postgres.sql` and `db/schema/sqlite.sql` keep the `orgs`
  column list comma-aligned; the new `org_protected` line is fine but
  matches both styles. No change needed.
- The `applyPlaygroundEnv` boolean-parse comment in `config.go`
  documents the divergence from `AuthRateLimitEnabled` — good. Could
  later be hoisted into a shared `parseBoolEnv` helper if a third
  bool env joins the file. Not worth doing for two.

**Notes**:
- Build, all package tests, `go vet` re-confirmed at review time.
- Foundation docs (`docs/SPEC.md`, `docs/ARCHITECTURE.md`) were rolled
  forward at scope time and continue to match the implementation
  (`Reserved orgs` paragraph at ARCHITECTURE.md L521 still accurate).
  `docs/SELF_HOST.md` § 15 added cleanly.
- Deviations from design (`store.ErrNotFound` vs `ErrNoRows`, extending
  existing `GetOrgBySlug` rather than adding a duplicate,
  `ListOrgsForAccount` column update for sqlc type compatibility) are
  all documented in implementation notes and sound.
- Parent epic `epic-ephemeral-playground` has 6 sibling features still
  at stage:review; epic auto-advance deferred until the remaining
  siblings clear.
