---
id: epic-portal-api-sessions-rest-session-invites-and-member-remove
kind: story
stage: done
tags: [portal, security]
parent: epic-portal-api-sessions-rest
depends_on: [epic-portal-api-sessions-rest-sessions-lifecycle]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
---

# Sessions REST — Session Invites and Member Remove

## Scope

Add the `session_invites` table + 3 endpoints (invite, accept, remove member).

## Units delivered

- `internal/db/migrations/{sqlite,postgres}/00007_session_invites.sql` — schema
- `db/schema/{sqlite,postgres}.sql` (edit)
- `db/queries/{sqlite,postgres}/session_invites.sql` — Insert/Get/MarkAccepted/ListPendingForSession
- Regen sqlitestore + pgstore
- `internal/db/store/store.go` (edit) — SessionInviteStore sub-interface
- Adapters updated
- `internal/portal/sessions/invites.go` — InviteToSession, AcceptSessionInvite handlers
- `internal/portal/sessions/members.go` — RemoveSessionMember handler
- `docs/openapi.yaml` (edit) — 3 paths + schemas Invite, InviteRequest, AcceptInviteRequest
- `cmd/portal/main.go` (edit) — register routes
- Tests

## Acceptance Criteria

- [ ] POST /api/sessions/<id>/invites: creator/member can invite; generates 32-byte token, hashed at rest, 7-day TTL; sends email via Sender
- [ ] POST /api/sessions/<id>/invites/<inviteID>/accept: validates token+email+expiry+not-already-accepted; creates session_member with role=member
- [ ] POST /api/sessions/<id>/members/<accountID>/remove: creator only; deletes session_members row; emits session-level event? (member-removed event is not in PROTOCOL.md's canon — skip for v1; the removed member's refs become read-only via pre-receive's existing membership check)
- [ ] Existing magic-link token model reused

## Notes

- Reuse the same `provision`-style flow as org invites where appropriate.
- Pre-receive already checks `GetSessionMember` on every push; a removed member's pushes are rejected automatically — no new policy code needed.
- Session-creation-time invitees (the `invitees` field in POST /api/sessions body) is handled by the lifecycle story implicitly: after creating the session, iterate `invitees` and call the same InviteToSession path internally. If lifecycle story didn't implement this, this story adds the helper + lifecycle invocation.

## Implementation notes

**Schema**: `session_invites` table added via migration `00007_session_invites.sql` (both sqlite and postgres). Mirrors the `org_invites` pattern: id, org_id, session_id, inviter_account_id, invitee_email, token_hash (UNIQUE), created_at, expires_at, accepted_at, accepted_by_account_id.

**Store**: `SessionInviteStore` interface added to `internal/db/store/store.go` with 5 methods (InsertSessionInvite, GetSessionInviteByID, GetSessionInviteByTokenHash, MarkSessionInviteAccepted, ListPendingSessionInvitesForSession). Both `sqliteAdapter`/`sqliteTxStore` and `postgresAdapter`/`postgresTxStore` implement it. sqlc generated the underlying queries.

**Handler**: `sessions.Handler` extended with `sender senders.Sender` and `portalURL string` fields. `sessions.New()` signature updated accordingly. `cmd/portal/main.go` passes `emailSender` and `cfg.PortalURL`.

**InviteToSession** (`internal/portal/sessions/invites.go`): verifies org+session membership, generates 32-byte token (sha256 hash at rest), inserts invite row (7-day TTL), sends email with accept URL `<portalURL>/sessions/<id>/invites/<inviteID>/accept?token=<raw>`. Returns `Invite` schema.

**AcceptSessionInvite**: Bearer-only. Verifies token hash, expiry, not-already-accepted, invitee_email case-insensitive match. Tx: MarkSessionInviteAccepted + AddSessionMember(role=member). Returns updated Session.

**RemoveSessionMember** (`internal/portal/sessions/members.go`): Creator-only. Guards against self-removal. Verifies target is a current member. Calls RemoveSessionMember. Returns 204.

**OpenAPI**: 3 new paths + 3 new schemas (Invite, InviteRequest, AcceptInviteRequest) added to `docs/openapi.yaml`. Go and TS types regenerated.

**Tests**: 10 integration tests in `internal/portal/sessions/invites_test.go` covering happy paths and all error conditions (403, 401, 404, 409). All pre-existing tests pass.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Mirrors the org-invites pattern. Tx for MarkAccepted + AddSessionMember prevents race. Self-removal guard is thoughtful. Pre-receive's existing membership check makes member-remove instantly enforced on git push.
