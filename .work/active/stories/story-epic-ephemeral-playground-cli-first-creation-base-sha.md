---
id: story-epic-ephemeral-playground-cli-first-creation-base-sha
kind: story
stage: implementing
tags: [portal]
parent: feature-epic-ephemeral-playground-cli-first-creation
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Post-receive `base_sha` stamping (portal side)

## Scope

Fixes the discovered gap surfaced during the parent feature's design
pass: the portal's `SetSessionBaseSHA()` store method exists but is
**never called in production**. The receive-pack post-receive handler at
`internal/portal/githttp/receive_pack.go` (lines 260-310) seeds the
draft ref from the base commit when the creator first pushes
`refs/heads/jam/<sessionID>/base`, but doesn't stamp `sessions.base_sha`
on the session row.

After this story lands, the CLI's `jamsesh new` flow leaves the session
in a complete state — `base_sha` is populated as soon as the base ref
push lands successfully.

This is Unit 10 from the parent feature's design body. It's a small,
self-contained server-side fix; isolating it as its own story lets it
land in parallel with the CLI-side stories.

Does NOT include: any CLI work, any handler-level refactor beyond the
post-receive stamping.

## Units delivered

1. `internal/portal/githttp/receive_pack.go` — add the
   `SetSessionBaseSHA` call after the existing draft-ref seeding logic
   (~20 LOC). Includes a small helper `findBaseRefUpdate(updates)` that
   scans the ref-updates list for a `.../base` ref.
2. `internal/portal/githttp/receive_pack_test.go` — extend (or add)
   tests per the parent feature's Testing section (3 test functions
   enumerated for Story C), running via the `stores(t)` dual-dialect
   harness.

## Acceptance criteria

- [ ] After a successful base-ref push, `sessions.base_sha` matches the
      pushed HEAD commit SHA (verified by SELECT after the push)
- [ ] A subsequent (non-base) ref push does NOT re-stamp `base_sha`
- [ ] If `SetSessionBaseSHA` fails (e.g. transient DB error injected via
      the test harness's wrapping store), the push still succeeds and
      a warning is logged — non-fatal degradation
- [ ] Multi-dialect: tests pass under SQLite (always) and Postgres
      (when `JAMSESH_TEST_PG_DSN` is set)
- [ ] No changes to the `pre-receive` validation path — the base-ref
      empty-repo gate (`internal/portal/prereceive/refs.go` lines
      30-114) is unchanged

## Notes for the implementing agent

- The `SetSessionBaseSHA` query is already defined in
  `db/queries/sqlite/sessions.sql` and `db/queries/postgres/sessions.sql`
  per the project's dual-dialect mirror pattern. Run `sqlc generate` to
  confirm the generated `Querier` interface includes `SetSessionBaseSHA`;
  if it doesn't, that's a separate fix (add the query, regenerate).
- The post-receive handler runs after the git subprocess exits cleanly,
  so there's no race with concurrent updates to this session — the
  receive-pack flow is single-threaded per push.
- Use `sql.NullString{String: <sha>, Valid: true}` for the nullable
  `base_sha` column (matches the existing sqlc-generated types).
- Logging: use the existing handler logger (`h.Logger` if available;
  check the package's conventions). Match the level used for other
  post-receive warnings in the same file.
- The `findBaseRefUpdate` helper should match `refs/heads/jam/<sessionID>/base`
  exactly (3 path components after `refs/heads/jam/`, last is `base`).
  Don't accidentally match user-refs like `refs/heads/jam/<id>/<user>/base`
  (a user choosing the literal branch name `base`) — match on the
  3-segment shape, not the trailing component alone.

## Cross-story note

Story `-new` (`jamsesh new` subcommand) does NOT declare a dependency on
this story. The CLI works end-to-end without `base_sha` stamped — the
session is functional, the user can fetch from it, the auto-merger
operates correctly. The stamping is correctness/observability, not
functional. So the orchestrator runs all three stories in parallel
without sequencing constraints.

If this story lands AFTER the `-new` story, sessions created in the
interval have `base_sha: NULL` (degraded but functional). A future
backfill query (out of scope here) could populate them; in practice
they're playground-or-pre-launch sessions so this doesn't matter.
