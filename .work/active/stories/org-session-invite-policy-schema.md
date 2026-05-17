---
id: org-session-invite-policy-schema
kind: story
stage: implementing
tags: [portal, security]
parent: org-session-invite-policy
depends_on: []
release_binding: null
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

- [ ] Migration applies cleanly on both dialects (SQLite + Postgres test
      DBs in `internal/db/store/*_test.go`)
- [ ] `make generate-db` (or `sqlc generate`) regenerates without errors
- [ ] Existing org-creation tests pass — new orgs default to
      `session_invite_policy = 'members_only'`
- [ ] `Store.GetOrg(...)` returns the new field populated for existing rows
- [ ] `Store.UpdateOrgSessionInvitePolicy(...)` round-trips correctly
- [ ] `docs/ARCHITECTURE.md` (or the new section's home) reflects the
      membership model
- [ ] `go build ./...` clean
- [ ] `go test ./internal/db/...` passes

## Risk

LOW. Additive schema change with a default; no breaking surface. The doc
update is informative, not normative — the code is the source of truth.

## Rollback

`git revert` the commit. If the migration has already been applied to a
real DB, run the `.down.sql` manually. For test DBs (created fresh each
run), no manual rollback needed.
