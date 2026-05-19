---
id: org-session-invite-policy-schema
kind: story
stage: done
tags: [portal, security]
parent: org-session-invite-policy
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Schema — `orgs.session_invite_policy` column + sqlc regen + foundation doc

Adds the policy column to the `orgs` table on both dialects, regenerates sqlc,
wires the new field through the store adapters, and rolls forward
`docs/ARCHITECTURE.md` with the membership-model subsection.

## Files

- New migrations: `internal/db/migrations/<NNN>_org_session_invite_policy.up.sql`
  and `<NNN>_org_session_invite_policy.down.sql` — verify naming and
  number-prefix convention by inspecting existing files in that directory.
- Source-of-truth schema for sqlc: `db/schema/*.sql` (check `sqlc.yaml` for
  exact path).
- New queries: `db/queries/orgs.sql` — add `GetOrgSessionInvitePolicy :one`
  and `UpdateOrgSessionInvitePolicy :exec`. Existing `SELECT *` org queries
  automatically pick up the new column on regeneration.
- Regenerated: `internal/db/sqlitestore/orgs.sql.go`, `internal/db/pgstore/orgs.sql.go`,
  plus the corresponding models.
- Update: `internal/db/store/store.go` — add `SessionInvitePolicy string` to
  the `Org` domain struct; add `UpdateOrgSessionInvitePolicy` method to the
  `Store` interface; add new params struct if sqlc emits one.
- Update: `internal/db/store/postgres_adapter.go` and `sqlite_adapter.go` —
  map the new column through `pgOrg(...)` / `sqliteOrg(...)` row mappers and
  implement the new store method.
- Update: `docs/ARCHITECTURE.md` — add the membership-model subsection (see
  parent feature body for required content).

## Target state

### Migration SQL

```sql
-- up
ALTER TABLE orgs
  ADD COLUMN session_invite_policy TEXT NOT NULL DEFAULT 'members_only'
  CHECK (session_invite_policy IN ('members_only', 'open'));

-- down
ALTER TABLE orgs DROP COLUMN session_invite_policy;
```

SQLite limitation: older SQLite versions struggle with `ADD COLUMN` plus
`CHECK`. Modern SQLite (3.37+) supports it; the project uses
`modernc.org/sqlite` which is sufficient. If the local SQLite migration
pattern is to use `CREATE TABLE ... ; INSERT INTO ... SELECT FROM old` for
constraint changes, follow that pattern instead — verify by reading the
most recent migration file.

### Domain type

```go
// internal/db/store/store.go (Org struct excerpt)
type Org struct {
    // ... existing fields ...
    SessionInvitePolicy string  // 'members_only' or 'open'
}
```

### sqlc queries

```sql
-- name: GetOrgSessionInvitePolicy :one
SELECT session_invite_policy FROM orgs WHERE id = ?;
-- (use $1 for Postgres syntax per existing query conventions)

-- name: UpdateOrgSessionInvitePolicy :exec
UPDATE orgs SET session_invite_policy = ? WHERE id = ?;
```

### Foundation doc update

Append a new section to `docs/ARCHITECTURE.md` (or create one in
`docs/SECURITY.md` if that file exists and is a better home — check both):

```markdown
## Membership model

Every persisted entity carries `org_id`. Every API route is org-scoped.
Two membership tables exist independently:

- **`org_members`** — the canonical org-level membership. Created by
  `CreateOrg` (creator role) and by `AcceptOrgInvite` (member role).
- **`session_members`** — per-session membership for the actor and any
  invitees. Created when a session is created (creator) and when an
  `AcceptSessionInvite` succeeds (member role).

The relationship between the two is governed by per-org policy:
`orgs.session_invite_policy`:

- **`members_only`** (default) — `AcceptSessionInvite` rejects unless the
  accepting account is already in `org_members` for the same org. Session
  membership implies org membership.
- **`open`** — `AcceptSessionInvite` succeeds regardless of org
  membership. The invitee becomes a session-scoped guest: in
  `session_members` for that session, but never auto-added to
  `org_members`. `handlerauth.RequireOrgMember` correctly keeps such
  guests out of org-scoped operations.

The gate fires at invite-accept time, not at every request. Once a
session_members row exists, the policy was enforced at the perimeter
and downstream handlers trust the membership.
```

## Acceptance criteria

- [x] Migration applies cleanly on both dialects (SQLite + Postgres test
      DBs in `internal/db/store/*_test.go`)
- [x] `make generate-db` (or `sqlc generate`) regenerates without errors
- [x] Existing org-creation tests pass — new orgs default to
      `session_invite_policy = 'members_only'`
- [x] `Store.GetOrg(...)` returns the new field populated for existing rows
- [x] `Store.UpdateOrgSessionInvitePolicy(...)` round-trips correctly
- [x] `docs/ARCHITECTURE.md` (or the new section's home) reflects the
      membership model
- [x] `go build ./...` clean
- [x] `go test ./internal/db/...` passes

## Implementation notes

**Greenfield** (no land mode — migrations 00014 did not exist).

### What was implemented

1. **Migration files** (`internal/db/migrations/{postgres,sqlite}/00014_session_invite_policy.sql`)
   — goose Up: `ALTER TABLE orgs ADD COLUMN session_invite_policy TEXT NOT NULL DEFAULT 'members_only' CHECK (session_invite_policy IN ('members_only', 'open'))`. Both dialects use the same SQL; modern SQLite (modernc.org/sqlite 3.37+) supports ADD COLUMN with CHECK.

2. **Source-of-truth schemas** (`db/schema/{postgres,sqlite}.sql`) — column appended to the `orgs` CREATE TABLE definition.

3. **Query files** — updated `GetOrgByID` and `GetOrgBySlug` to include `session_invite_policy` in explicit SELECT lists; also updated `ListOrgsForAccount` in `org_members.sql` (which used an explicit column list and needed the new column to avoid a generated `ListOrgsForAccountRow` type mismatch); added `GetOrgSessionInvitePolicy :one` and `UpdateOrgSessionInvitePolicy :exec`.

4. **sqlc regen** — `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate` — clean. Also added missing sqlc.yaml overrides for `acquired_at`, `heartbeat_at`, `last_activity_at`, and `released_at` timestamp columns that were present in the committed generated code (needed for the leases feature) but missing from sqlc.yaml — fixing pre-existing build errors from the leases story.

5. **Domain wiring** (`internal/db/store/store.go`) — added `SessionInvitePolicy string` to `Org`; added `UpdateOrgSessionInvitePolicy` to `OrgStore` interface and `TxStore`; declared `UpdateOrgSessionInvitePolicyParams`.

6. **Adapters** (`postgres_adapter.go`, `sqlite_adapter.go`) — updated `pgOrg`/`sqliteOrg` row mappers; implemented `UpdateOrgSessionInvitePolicy` on both outer adapters and both TxStore types.

7. **Foundation doc** — added "## Membership model" section to `docs/ARCHITECTURE.md` (chosen over `docs/SECURITY.md` because the section explains data layer policy semantics, fitting the existing "Data layer" section in ARCHITECTURE; SECURITY.md focuses on auth/trust boundaries).

### Pre-existing bug fixes (test debt, not product bugs)

- **sqlc.yaml missing overrides** — `acquired_at`, `heartbeat_at`, `last_activity_at`, `released_at` were not in sqlc.yaml overrides but the committed generated code had `time.Time` types (generated by an older sqlc version). Running sqlc v1.31.1 without those overrides produced `pgtype.Timestamptz`/`string` mismatches in the leases and finalize_locks adapter code. Fixed by adding the overrides — restoring the correct types.
- **Test stubs** — several portal package test files had stub implementations of `openapi.StrictServerInterface` missing the `GetSessionInvite` method (added by the in-progress session-invites feature). Fixed stale stubs in: `comments/service_test.go`, `accounts/handlers_test.go`, `auth/magic_link_test.go`, `auth/oauth_test.go`, `tokens/handlers_test.go`, `handlerauth/handlerauth_test.go`.

### Verification

- `go build ./...` — clean
- `go test ./internal/db/...` — all pass
- `go test ./internal/portal/...` — all pass

## Risk

LOW. Additive schema change with a default; no breaking surface. The doc
update is informative, not normative — the code is the source of truth.

## Rollback

`git revert` the commit. If the migration has already been applied to a
real DB, run the `.down.sql` manually. For test DBs (created fresh each
run), no manual rollback needed.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Story changes spread across two commits — schema work landed in 9f668a6 but
  the postgres_adapter/sqlite_adapter wiring committed in ef09ad0
  (get-invite-details) because that agent committed first. Final state is
  correct; just a history wrinkle from parallel orchestration.
- No dedicated `Store.UpdateOrgSessionInvitePolicy` round-trip test — the
  method is exercised indirectly via `TestPatchOrg_CreatorSuccess` and
  `TestPatchOrg_Grandfather` in the downstream patch-endpoint story.
- The agent's bonus `sqlc.yaml` overrides for lease/finalize-lock timestamp
  columns are unrelated cleanup; commendable and harmless.

**Notes**: Migration is additive with a DEFAULT, so non-breaking. The CHECK
constraint provides defense-in-depth at the DB boundary. ARCHITECTURE.md
membership-model section reads cleanly and describes the system as it now
behaves. Build + tests pass across the full portal suite.
