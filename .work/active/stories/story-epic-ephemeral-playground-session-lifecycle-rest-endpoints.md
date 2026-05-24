---
id: story-epic-ephemeral-playground-session-lifecycle-rest-endpoints
kind: story
stage: implementing
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
