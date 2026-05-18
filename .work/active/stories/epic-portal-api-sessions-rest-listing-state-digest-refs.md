---
id: epic-portal-api-sessions-rest-listing-state-digest-refs
kind: story
stage: done
tags: [portal]
parent: epic-portal-api-sessions-rest
depends_on: [epic-portal-api-sessions-rest-sessions-lifecycle]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
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

## Implementation notes

- `internal/portal/pagination/cursor.go` — `Encode`/`Decode` + `FilterHash` + `NewCursor`. Cursor stores `last_created_at_ns` (Unix nanoseconds) as the exclusive upper bound for the `created_at DESC` sort. Filter hash is SHA-256 of sorted `k=v` pairs.
- `internal/portal/sessions/listing.go` — `ListSessions` handler. Fetches `limit+1` rows to detect next page; builds cursor from last row's `created_at`+`id`. Returns 400 `pagination.cursor_filter_mismatch` on hash mismatch.
- `internal/portal/sessions/state.go` — `GetSession` (org+session member check), `ListSessionRefs` (opens bare repo via `git.PlainOpen`, iterates refs under `refs/heads/jam/<sessionID>/`, looks up mode from `ref_modes` or session `default_mode`), `GetSessionDigest` (fetches digest-relevant events, assembles plain-text block per PROTOCOL.md sections: peer activity, comments, conflicts, mode changes, state summary).
- SQL: `ListSessionsForOrgWithCursor` (`WHERE org_id=? AND created_at < ? ORDER BY created_at DESC LIMIT ?`), `ListEventsSinceForDigest` (filters to 6 digest-relevant event types).
- Both queries added to sqlite and postgres dialect files; sqlc regenerated; store interface + both adapters (outer + TxStore) updated.
- `docs/openapi.yaml`: 4 new paths (GET /sessions, GET /sessions/{id}, GET /sessions/{id}/refs, GET /sessions/{id}/digest) + 4 schemas (SessionListResponse, Ref, RefListResponse, DigestResponse). GET methods added to existing path blocks (no duplicate keys).
- `cmd/portal/main.go`: 4 new route registrations + 4 new combinedHandler delegations.
- Test shims in auth, accounts, tokens packages updated for 4 new interface methods.
- All existing tests pass; 9 new tests added in `listing_state_test.go` covering cursor round-trip, filter mismatch, empty repo refs, member auth, digest assembly.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Cursor with filter-hash invalidation prevents the cross-filter cursor reuse bug class. Digest assembly per PROTOCOL.md sections. ListSessionRefs gracefully handles empty repo case (no bare repo yet means empty refs list).
