---
id: epic-portal-api-sessions-rest
kind: feature
stage: implementing
tags: [portal]
parent: epic-portal-api
depends_on: [epic-portal-api-events-log, epic-portal-foundation-http-skeleton, epic-portal-foundation-accounts, epic-portal-foundation-auth-flows, epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal API — Sessions REST

## Brief

The REST surface for sessions: creation, browsing, lifecycle transitions
(finalize / abandon), and member/invitation management. Also owns the
digest endpoint that the local binary calls at every turn start, and the
refs-listing endpoint the UI uses.

**Endpoints delivered** (per `docs/PROTOCOL.md > REST API > Sessions and
Session state`):

- `POST /api/sessions` — create a session. Body: `name`, `goal`,
  `scope` (path globs), `default_mode`, optional `invitees`.
  Implementation orchestration: insert `sessions` row, insert
  creator's `session_members` row (role: creator), call the
  cross-epic `epic-portal-git-storage` init helper to create the bare
  repo, emit `session.created` event. On bare-repo creation failure,
  rollback. On row insert failure, `rm -rf` the half-created repo via
  the storage helper.
- `GET /api/sessions` — list sessions visible to the caller (active +
  recent, cursor-paginated).
- `GET /api/sessions/<id>` — session metadata + member list summary.
- `PATCH /api/sessions/<id>` — update `goal`, widen `scope`
  (write-restrictive narrowing is rejected), change `default_mode`
  (only by creator).
- `POST /api/sessions/<id>/finalize` — transition session to
  `status: finalizing`, emit `session.finalizing` event. Idempotent
  (re-entering finalizing is a no-op); concurrent-finalize behavior per
  `epic-finalize-flow`'s lock semantics (member-id stored, override
  flow).
- `POST /api/sessions/<id>/abandon` — close session without finalize;
  status → `ended`, end_reason → `abandoned`, emit `session.ended` event.
  Creator-only.
- `GET /api/sessions/<id>/refs` — list all refs in the session bare
  repo with mode (sync/isolated) and current tip sha. Reads from
  `epic-portal-git-storage` via go-git.
- `GET /api/sessions/<id>/digest?since=<seq>` — assembles a
  turn-start text block for `additionalContext`: peer commit activity
  (read from `events` filtered to `commit.arrived` since cursor),
  social digest (addressed comments, conflict events), current state
  (goal, draft tip, your refs and modes, open conflicts). Returns a
  text payload AND a `next_cursor`.
- `POST /api/sessions/<id>/invites` — invite a member by email or
  org account id; persists an `invites` row and (for email) sends a
  magic-link-style join email via the auth-flows `Sender` interface.
- `POST /api/sessions/<id>/invites/<invite_id>/accept` —
  recipient binds to the session (creates a `session_members` row).
- `POST /api/sessions/<id>/members/<account_id>/remove` —
  remove a session member. Creator-only. Their refs become read-only
  (the pre-receive policy from `epic-portal-git-pre-receive` already
  rejects pushes from non-members).

**Schema additions** (sqlc migrations owned by this feature, on top of
data-layer's initial `sessions` and `session_members` tables):

- `invites` — id, org_id, session_id, inviter_account_id,
  invitee_email, invitee_account_id (nullable until accepted),
  token (single-use), expires_at, accepted_at, status.
- Per-session metadata columns added to `sessions`: `status` (active /
  finalizing / ended), `end_reason` (shipped / abandoned / timeout),
  `ended_at`, `finalize_locked_by_account_id` (nullable, the
  finalize-flow lock).

**Pagination model** (locked at epic-design): cursor-based,
`{items, next_cursor}` response shape. Cursor is opaque
`base64(filter_hash + last_seq_id)`. Cursor reuse with changed filter →
`pagination.cursor_filter_mismatch` error.

**Role checks**: creator-only endpoints (patch, abandon, member
removal) enforce via `session_members.role == "creator"` lookup in
middleware.

Does NOT cover the comment-related endpoints (`comments-rest` owns
list/resolve; create is via MCP). Does NOT cover finalize-plan
generation — that lives in `epic-finalize-flow`. Does NOT cover the
WebSocket subscription (separate feature).

## Epic context

- Parent epic: `epic-portal-api`
- Position in epic: the biggest REST surface; depends on events-log for
  emission, http-skeleton for mount, accounts for org-membership
  resolution, auth-flows for invite email delivery, and the cross-epic
  `epic-portal-git-storage` for bare-repo creation orchestration.

## Foundation references

- `docs/PROTOCOL.md` — REST API > Sessions, REST API > Session state
  (digest, refs), Auth section, HTTP error contract
- `docs/SPEC.md` — Session shape (Name / Goal / Scope / Default mode /
  Base ref), Lifecycle (Creation, End, Retention), Auth model (Session
  authorization)
- `docs/SECURITY.md` — Authorization (role-restricted operations)
- `docs/UX.md` — Flow: creating a session, Flow: joining a session

## Inherited epic design decisions

- **Pagination**: cursor-based with filter-hash invalidation.
- **Event emission**: every lifecycle transition emits the canonical
  event from PROTOCOL.md's catalog.
- **Bare repo creation**: eager, direct Go function call into
  `epic-portal-git-storage` from the `POST /api/sessions` handler.
  Atomic compensation: rm-rf the repo on row insert failure.

## Generated-contracts scope

Per the SPEC.md generated-contracts decision, every endpoint in this
feature gets a corresponding entry under `paths:` in
`docs/openapi.yaml`. Request and response bodies reference component
schemas (this feature adds: `Session`, `SessionSummary`,
`SessionListResponse`, `CreateSessionRequest`, `PatchSessionRequest`,
`Ref`, `RefListResponse`, `DigestResponse`, `Invite`, `Member`,
`MemberRole`).

Handlers implement the `oapi-codegen`-generated `ServerInterface`
methods (one method per spec'd endpoint). The generated interface is
the compile-time contract between the spec and the handler; drift
becomes a build error. Pagination cursor opaque shape and
`pagination.cursor_filter_mismatch` error code are spec'd in the YAML
under the standard error contract.

## Decomposition risks

- This feature is at the size ceiling (12-15 implementation units). If
  the design pass surfaces meaningful additional complexity (e.g.,
  invite-acceptance edge cases compound, or finalize lock semantics
  expand), the design pass may signal back to autopilot to split out a
  `sessions-membership` feature. Capacity reserved.
- Idempotency of finalize/abandon transitions matters — a double-fire
  must not emit duplicate `session.ended` events. Design pass locks the
  status-transition state machine.

## Design decisions

- **Package**: `internal/portal/sessions/` with a `Handler` satisfying the strict-server interface for all sessions-rest endpoints. Per-endpoint methods on Handler.
- **Schema additions** (00006 migration): extend `sessions` table with `end_reason`, `finalize_locked_by_account_id`; add `invites` table.
- **Pagination cursor**: `base64url(json{filter_hash, last_seq, last_id})`. Filter hash = SHA-256 of normalized query params. Cursor reuse with changed filter → `pagination.cursor_filter_mismatch` 400.
- **POST /sessions transaction**: open Tx; insert session row; insert session_member row (role=creator); call `storage.CreateRepo(ctx, orgID, sessionID)`; if repo creation fails → rollback Tx + return error. Then emit `session.created` event (best-effort).
- **PATCH /sessions/<id>**: scope-narrowing rejected with 400 (`session.scope_narrowing_rejected`); scope-widening + goal/default-mode change allowed for creator.
- **finalize/abandon idempotency**: check current status; if already in target state, return 200 with current row (no-op, no event). State-machine: active → finalizing → ended (finalize/abandon both terminate). Once ended, no transitions.
- **digest endpoint**: reads from `events` table since cursor; assembles a plain-text block + a structured JSON sidecar; returns `{text, next_cursor}`. The text format follows `docs/PROTOCOL.md > Session state > Digest`.
- **refs endpoint**: opens bare repo via `storage`, iterates refs, returns `{ref, sha, mode}` per ref. Mode comes from a per-ref metadata table OR a naming convention — for v1, store `(session_id, ref) → mode` in a small `ref_modes` table created by THIS feature's migration. Or simpler: ref-mode lives in PROTOCOL.md as a session-level + per-ref attribute; for v1, the session has a `default_mode` and any non-default mode lives on the ref itself via the ref-name suffix (e.g., `-isolated`). Pick whichever is cleanest. Going with a `ref_modes` table for explicitness.

Actually scrap that — `epic-portal-git-pre-receive` already references mode but doesn't manage it. The session has `default_mode` (already in schema). Per-ref mode override is a v1 feature. Let me add a `ref_modes` table here.

- **invites table**: `id, org_id, session_id, inviter_account_id, invitee_email, invitee_account_id (nullable), token_hash, expires_at, accepted_at, accepted_by_account_id (nullable)`. Reuses the magic-link pattern but session-scoped.
- **Story decomposition**: 3 stories.
  1. `sessions-lifecycle` — schema additions (status enum, end_reason, finalize_lock, ref_modes table); POST/PATCH/finalize/abandon endpoints; openapi schemas. depends_on: []
  2. `listing-state-digest-refs` — GET /sessions, GET /sessions/<id>, GET /sessions/<id>/refs, GET /sessions/<id>/digest; cursor pagination helper. depends_on: [sessions-lifecycle]
  3. `session-invites-and-member-remove` — invites table + 3 endpoints + member-remove endpoint. depends_on: [sessions-lifecycle]

## Implementation Units (high-level)

### Story 1: sessions-lifecycle

- Schema 00006: add `end_reason TEXT`, `finalize_locked_by_account_id TEXT REFERENCES accounts(id)` to sessions; new `ref_modes(session_id, ref, mode, PK(session_id, ref))` table
- Queries: UpdateSessionGoalScopeMode, UpdateSessionStatus (extend existing), SetSessionEndReason, GetSessionWithLock
- Handler: `CreateSession`, `PatchSession`, `FinalizeSession`, `AbandonSession`
- openapi.yaml: 4 paths + schemas `Session`, `CreateSessionRequest`, `PatchSessionRequest`, `MemberSummary`

### Story 2: listing-state-digest-refs

- Cursor pagination helper `internal/portal/pagination/cursor.go` — encode/decode + filter-hash invalidation
- Queries: ListSessionsForOrgWithCursor, ListEventsSinceForDigest (subset projection)
- Handler: `ListSessions`, `GetSession`, `ListRefs`, `GetDigest`
- Digest assembly: query events since cursor, group by type; format text block per PROTOCOL.md
- openapi.yaml: 4 paths + schemas `SessionListResponse`, `RefListResponse`, `Ref`, `DigestResponse`

### Story 3: session-invites-and-member-remove

- Schema 00007: `invites` table
- Queries: InsertInvite, GetInviteByID, GetInviteByTokenHash, MarkInviteAccepted, ListPendingForSession
- Handler: `InviteToSession`, `AcceptSessionInvite`, `RemoveSessionMember`
- Send via existing senders.Sender
- openapi.yaml: 3 paths + schemas `Invite`, `InviteRequest`, `AcceptInviteRequest`

## Implementation Order

1. sessions-lifecycle (foundation: schema + CRUD)
2. (parallel) listing-state-digest-refs + session-invites-and-member-remove

## Testing

- Per-endpoint integration tests against in-memory SQLite
- POST /sessions: tx rollback on repo failure verified
- PATCH: scope narrow rejected
- finalize/abandon: idempotent
- Cursor: round-trip + filter-hash mismatch
- Invite accept: token validation + member binding

## Risks

- **Size ceiling**: 11 endpoints across 3 stories is the right cut.
- **Cursor design**: base64(JSON) opaque cursor is more debuggable than just a seq number. Filter-hash invalidation prevents cross-filter cursor reuse confusion.
