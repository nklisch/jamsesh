---
id: epic-portal-api-sessions-rest-listing-state-digest-refs
kind: story
stage: implementing
tags: [portal]
parent: epic-portal-api-sessions-rest
depends_on: [epic-portal-api-sessions-rest-sessions-lifecycle]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Sessions REST — Listing, State, Digest, Refs

## Scope

Add the 4 read endpoints: list sessions, get session, list refs, get digest. Plus the cursor-pagination helper.

## Units delivered

- `internal/portal/pagination/cursor.go` — Encode/Decode + FilterHash
- `internal/portal/sessions/listing.go` — ListSessions handler
- `internal/portal/sessions/state.go` — GetSession, ListRefs, GetDigest handlers
- `db/queries/{sqlite,postgres}/sessions.sql` (edit) — ListSessionsForOrgWithCursor
- `db/queries/{sqlite,postgres}/events.sql` (edit) — ListEventsSinceForDigest (selecting only digest-relevant types)
- Regen
- `docs/openapi.yaml` (edit) — 4 paths + schemas SessionListResponse, RefListResponse, Ref, DigestResponse
- `cmd/portal/main.go` (edit) — register routes
- Tests

## Acceptance Criteria

- [ ] GET /api/sessions: cursor round-trip; filter-hash mismatch → 400 `pagination.cursor_filter_mismatch`
- [ ] GET /api/sessions/<id>: returns Session + member-summary array
- [ ] GET /api/sessions/<id>/refs: opens bare repo via storage, lists refs with mode from ref_modes table (defaults to session.default_mode when no override)
- [ ] GET /api/sessions/<id>/digest?since=<seq>: assembles text block + next_cursor; events filtered to digest-relevant types
- [ ] Digest text follows PROTOCOL.md sections: peer commits, comments, conflicts, mode changes, state summary
- [ ] All routes Bearer + RequireOrgRole(creator|member) gated

## Notes

- Cursor format: base64url(json{filter_hash, last_seq, last_id}). On request, decode and verify filter_hash matches a recomputed hash of the current query params.
- The digest's "current state" section reads session metadata + your-refs from session_members + ref_modes.
