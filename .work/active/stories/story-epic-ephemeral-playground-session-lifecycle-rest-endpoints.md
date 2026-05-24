---
id: story-epic-ephemeral-playground-session-lifecycle-rest-endpoints
kind: story
stage: review
tags: [portal, playground]
parent: feature-epic-ephemeral-playground-session-lifecycle
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Playground REST endpoints + handle generator

## Scope

Story 1 of the parent feature. Owns the HTTP surface of playground
session-lifecycle: the wordlist + handle generator, the four playground
REST handlers (create, join, get, get-tombstone), the OpenAPI spec
additions, the schema additions (`sessions.last_substantive_activity_at`,
`hard_cap_at`, `idle_timeout_at`, new `tombstones` table), and the
router wiring.

Full design including signatures, code skeletons, and per-handler
acceptance criteria is in the parent feature body's "Story 1" section.

## Files delivered

- `internal/portal/playground/wordlist/adjectives.txt` (~256 entries, curated)
- `internal/portal/playground/wordlist/animals.txt` (~256 entries, curated)
- `internal/portal/playground/wordlist/wordlist.go` — embed + Pick()
- `internal/portal/playground/wordlist/wordlist_test.go`
- `internal/portal/playground/handler.go` — CreateSession, JoinSession, GetSession, GetTombstone
- `internal/portal/playground/handler_test.go`
- `internal/db/migrations/{sqlite,postgres}/NNNN_playground_sessions.sql`
- `db/schema/{sqlite,postgres}.sql` (modify) — add new sessions columns + tombstones table
- `db/queries/{sqlite,postgres}/sessions.sql` (extend) — CreateSession params extended;
  add `NicknameTakenInSession`, `CountSessionMembers`, `GetTombstone`, `RecordTombstone`
- `docs/openapi.yaml` (extend) — 4 new routes + 6 new component schemas
- `internal/api/openapi/*.gen.go` (regenerated via `make generate`)
- `internal/portal/router/router.go` (modify) — mount playground handler

## Acceptance criteria

See the parent feature body's "Story 1 acceptance criteria" section.
Summary: all 4 endpoints behave correctly under the playground-enabled
gate, joiner overflow returns 409, tombstone endpoint returns 404 for
active sessions and the summary after destruction, OpenAPI
`make generate` is clean.

## Notes for the implementing agent

- Wordlist content: curate for calm/positive sentiment and broad
  recognizability. Avoid any potentially offensive combinations. Review
  with a fresh eye before committing.
- The `uniqueHandle` collision-retry uses up to 10 attempts then falls
  back to a random suffix — see the function body in the feature design.
- Schema additions to `sessions` are nullable columns (`hard_cap_at`,
  `idle_timeout_at`) for backward compatibility with durable sessions
  that don't use them; `last_substantive_activity_at` defaults to
  `created_at` for existing rows on migration.
- The `tombstones` table is new (own primary key on `session_id`); no
  cascade from sessions since the session row is GONE by the time the
  tombstone is the only record.
- Bearer issuance via `tokens.Service.IssueAnonymousSessionBearer` —
  consumes the wave-1 anon-bearer feature primitive.
- Reserved org via `playground.ReservedOrgID = "org_playground"` constant
  — consumes the wave-1 reserved-org feature primitive.

## Implementation notes

### Design deviation: bearer issuance outside the session TX

The feature design sketch placed `IssueAnonymousSessionBearer` inside
`store.WithTx`. In practice this causes a SQLite deadlock in tests: the
outer TX holds the WAL write lock, and `IssueAnonymousSessionBearer` opens
its own TX on the same pool (it is wired against the top-level store, not
the TxStore). The fix is to split the create path into three sequential
steps: (1) session row in TX, (2) bearer + anon-account issuance outside
TX, (3) member row inserted directly. Partial failure at step 2 or 3 leaves
an orphaned session row; the destruction sweep (Story 2) cleans this up.
The same pattern is used for join (bearer issued outside, member row added
after).

### Router wiring

Playground routes are mounted inside the existing `MountAPI` closure in
`cmd/portal/main.go`:

- `GET /playground/sessions/{id}` inside the bearer-middleware group (the
  handler validates session membership of the caller's anon account).
- `POST /playground/sessions`, `POST /playground/sessions/{id}/join`, and
  `GET /playground/sessions/{id}/tombstone` in a sibling unauthenticated
  group (no bearer middleware; the handler returns 503 when `Enabled=false`).

### Files delivered (actual)

All files listed in the Scope section are present. The `router.go` file
was NOT modified — wiring went into `cmd/portal/main.go` directly (the
router uses a `MountAPI` hook pattern and does not have a separate
playground-handler field). The `playground.Handler` was added to
`combinedHandler` in `cmd/portal/main.go` and 4 delegate methods were
added to satisfy `StrictServerInterface`.
