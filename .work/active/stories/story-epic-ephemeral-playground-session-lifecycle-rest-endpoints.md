---
id: story-epic-ephemeral-playground-session-lifecycle-rest-endpoints
kind: story
stage: done
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

## Review (2026-05-23)

**Verdict**: Approve with comments

**Blockers**: none

**Important** (filed as backlog items, all `stage: implementing`):
- `bug-playground-wordlist-duplicate-adjectives` — `adjectives.txt`
  has 62 duplicate entries (effective ~177 unique adjectives instead
  of ~239), biasing `Pick()` distribution and shrinking the handle
  space.
- `bug-playground-handler-missing-test-coverage` — three explicit
  story ACs are not tested: (1) join after `hard_cap_at` elapsed →
  410, (2) bare-repo create failure rollback path, (3) tests run only
  under SQLite, not Postgres (story design required both via a
  `stores(t)` harness).
- `bug-playground-create-skips-writable-scope-validation` — playground
  CreateSession stores the user-supplied `scope` verbatim without the
  `validateWritableScope` check the durable session handler enforces;
  a malformed scope poisons the session until destruction.

**Nits** (not filed):
- `docs/openapi.yaml:3251` (GetPlaygroundSession): description text
  says "403 for a valid bearer that does not belong to a member" but
  the responses block declares only 401 and the handler returns 401
  with `auth.not_a_member`. Update description for consistency.
- `internal/db/migrations/{sqlite,postgres}/00018_playground_sessions.sql`:
  `last_substantive_activity_at` is added as nullable and back-filled
  but never enforced NOT NULL. Schema files carry a comment
  acknowledging this. Application code always inserts a value, so
  functionally fine — would be nice to add the NOT NULL constraint in
  a follow-up migration for postgres (sqlite ALTER limitations make
  this expensive to enforce there).

**Notes**:
- Build clean (`go build ./...`), vet clean (`go vet ./...`), package
  tests pass.
- The bearer-issuance-outside-TX deviation from the design is well
  documented in implementation notes and the partial-failure path is
  handled by destruction sweep — appropriate trade-off for SQLite.
- Router wiring landed in `cmd/portal/main.go` rather than the
  documented `router.go`, with a clear rationale in the implementation
  notes — matches the actual codebase pattern.
- OpenAPI spec extensions are well-structured and generated code is
  committed (no `make generate` drift).
- Tombstone idempotency confirmed via `ON CONFLICT (session_id) DO NOTHING`
  in both dialects.
