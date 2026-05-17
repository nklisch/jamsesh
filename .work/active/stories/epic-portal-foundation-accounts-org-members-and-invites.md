---
id: epic-portal-foundation-accounts-org-members-and-invites
kind: story
stage: done
tags: [portal, security]
parent: epic-portal-foundation-accounts
depends_on: [epic-portal-foundation-accounts-me-and-org-create]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Accounts — Org Members and Invites

## Scope

Add the `org_invites` table + the 3 admin endpoints: list members,
create invite, accept invite.

## Units delivered

- `internal/db/migrations/{sqlite,postgres}/00005_org_invites.sql`
- `db/schema/{sqlite,postgres}.sql` (edit — append `org_invites`)
- `db/queries/{sqlite,postgres}/org_invites.sql` — Insert, GetByID, GetByTokenHash, MarkAccepted, ListPendingForOrg, ListPendingForEmail
- Regen sqlitestore + pgstore
- `internal/db/store/store.go` (edit) — `OrgInviteStore` sub-interface
- Both adapters updated
- `internal/portal/accounts/orgs.go` — `ListMembersHandler`, `CreateInviteHandler`, `AcceptInviteHandler`
- `docs/openapi.yaml` (edit) — 3 paths + 4 schemas (`MemberRef`, `InviteBody`, `InviteRef`, `AcceptInviteBody`)
- Regen openapi
- `cmd/portal/main.go` (edit) — register routes with appropriate role middleware
- Tests

## Acceptance Criteria

- [ ] `GET /api/orgs/<org_id>/members` requires `creator` or `member` role; returns array of `{account_id, email, display_name, role, joined_at}`
- [ ] `POST /api/orgs/<org_id>/invites` requires `creator`; accepts `{email}`; generates 32-byte token, stores hash + 7-day TTL; sends email via Sender (subject mentions org name + inviter); returns `InviteRef` with `invite_id` (NOT the raw token — the token is only in the email)
- [ ] `POST /api/orgs/<org_id>/invites/<invite_id>/accept` requires Bearer; accepts `{token}` in body; verifies token hash matches, recipient email matches authenticated account email, not expired, not already accepted; binds account as `member`; marks invite accepted; returns the org
- [ ] Expired token → 401 `auth.invalid_token`
- [ ] Already-accepted token → 409 with appropriate error code
- [ ] Wrong recipient email → 403 `auth.insufficient_permission`

## Notes

- Token model is the same as magic-link: 32 random bytes hex, SHA-256 hashed at rest, raw token only in the email link.
- The email link in v0: `<portal_url>/orgs/<org_id>/invites/<invite_id>/accept?token=<raw>`. The SPA route handles the inbound and POSTs to the accept endpoint with the token in body.
- After this story lands, the foundation epic (`epic-portal-foundation`) has all 5 child features done and can advance to `done`.

## Implementation notes

- `internal/db/migrations/{sqlite,postgres}/00005_org_invites.sql` — schema with goose Up/Down
- `db/schema/{sqlite,postgres}.sql` — appended org_invites table definition
- `db/queries/{sqlite,postgres}/org_invites.sql` — 6 queries: Insert, GetByID, GetByTokenHash, MarkAccepted, ListPendingForOrg, ListPendingForEmail
- `sqlc.yaml` — added `accepted_at` column override for both engines
- Generated sqlitestore/pgstore: `org_invites.sql.go` + updated `models.go`, `querier.go`
- `internal/db/store/store.go` — `OrgInvite` domain type, 5 param types, `OrgInviteStore` interface; `OrgInviteStore` added to both `Store` and `TxStore`
- `internal/db/store/sqlite_adapter.go` / `postgres_adapter.go` — full adapter + TxStore delegate implementations
- `internal/portal/accounts/handlers.go` — Handler extended with `sender senders.Sender` + `portalURL string` fields; `New(...)` updated
- `internal/portal/accounts/orgs.go` — `ListOrgMembers`, `CreateOrgInvite`, `AcceptOrgInvite` strict-server methods
- `docs/openapi.yaml` — 4 new schemas (MemberRef, InviteBody, InviteRef, AcceptInviteBody) + 3 new paths
- Regenerated `internal/api/openapi/server.gen.go` and `frontend/src/lib/api/types.gen.ts`
- `cmd/portal/main.go` — `accounts.New(...)` updated; `ServerInterfaceWrapper` used for path-param routes; 3 new routes with appropriate role gates
- `internal/portal/accounts/orgs_test.go` — 7 tests covering all acceptance criteria
- Existing test stubs updated (magic_link_test, oauth_test, tokens/handlers_test, accounts/handlers_test) to satisfy the expanded `StrictServerInterface`

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Three-endpoint surface clean: list (read role), create (creator only), accept (Bearer only — the user is joining). Token model mirrors magic-link (raw in email, hash at rest). Atomic Tx for MarkAccepted + AddOrgMember prevents race where a second accept arrives between mark and bind.
