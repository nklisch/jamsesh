---
id: epic-portal-foundation-accounts-org-members-and-invites
kind: story
stage: implementing
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
