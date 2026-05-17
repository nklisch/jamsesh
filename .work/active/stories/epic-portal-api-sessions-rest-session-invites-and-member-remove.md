---
id: epic-portal-api-sessions-rest-session-invites-and-member-remove
kind: story
stage: implementing
tags: [portal, security]
parent: epic-portal-api-sessions-rest
depends_on: [epic-portal-api-sessions-rest-sessions-lifecycle]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
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
