---
id: epic-portal-api-sessions-rest
kind: feature
stage: drafting
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

## Decomposition risks

- This feature is at the size ceiling (12-15 implementation units). If
  the design pass surfaces meaningful additional complexity (e.g.,
  invite-acceptance edge cases compound, or finalize lock semantics
  expand), the design pass may signal back to autopilot to split out a
  `sessions-membership` feature. Capacity reserved.
- Idempotency of finalize/abandon transitions matters — a double-fire
  must not emit duplicate `session.ended` events. Design pass locks the
  status-transition state machine.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
