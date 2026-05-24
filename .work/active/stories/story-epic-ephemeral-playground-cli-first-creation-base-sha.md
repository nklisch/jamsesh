---
id: story-epic-ephemeral-playground-cli-first-creation-base-sha
kind: story
stage: done
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

## Implementation notes

### What was done

1. **`internal/portal/githttp/receive_pack.go`**: Added two changes:
   - Imported `jamsesh/internal/db/store`, `jamsesh/internal/portal/gitref`, and `strings`.
   - Added a `SetSessionBaseSHA` call immediately after the existing draft-ref seeding loop. The call is guarded by `findBaseRefUpdate(sessionID, updates)` returning non-nil.
   - Added `findBaseRefUpdate(sessionID string, updates []gitref.RefUpdate) *gitref.RefUpdate` helper at the top of the non-handler portion of the file. Matches `refs/heads/jam/<sessionID>/base` by exact string comparison; the `strings.Count(rest, "/") != 1` guard makes the intent explicit.

2. **`internal/portal/githttp/receive_pack_test.go`**: Added three tests at the end of the file:
   - `TestPostReceive_BaseRefStampsBaseSHA` — pushes `refs/heads/jam/<session>/base`, verifies `sessions.base_sha` equals the pushed SHA.
   - `TestPostReceive_NonBaseRefDoesNotReStamp` — seeds base_sha via a base-ref push, then pushes a user ref; verifies base_sha unchanged.
   - `TestPostReceive_SetBaseSHAFailureIsNonFatal` — injects a `passthroughStore` wrapper that fails `SetSessionBaseSHA`; verifies git push exits 0 and the warning appears in logs.
   - Also added the `passthroughStore` embedding wrapper (struct embeds `store.Store`, overrides only `SetSessionBaseSHA` via a function field).

### Deviations from design

- **`sql.NullString` vs `*string`**: The design skeleton used `sql.NullString{String: ..., Valid: true}` for `BaseSHA`. The actual `SetSessionBaseSHAParams.BaseSHA` field is `*string`. Used `&sha` instead.
- **`SetSessionBaseSHAParams.SessionID` vs `.ID`**: The design skeleton used `SessionID:` as the field name; the actual struct uses `ID:`. Fixed accordingly.
- **Logger field**: The design said to use `h.Logger` if available. The handler doesn't have a Logger field; the codebase uses `slog.WarnContext(r.Context(), ...)` throughout. Used that pattern.
- **`findBaseRefUpdate` takes `sessionID` as first arg**: The design didn't specify the signature; added `sessionID` so the function can do an exact match on the full ref string without needing the caller to reconstruct it. This makes the function usable in isolation.
- **Line numbers drifted from design**: The post-receive stamping code was inserted at lines ~280-300 (after the draft-ref seeding loop ending at line 280), not "lines 260-310" as the design body stated. The structure matched the design intent.

### Build and test status

- `go build ./internal/portal/githttp/... ./internal/db/store/...` — passes
- `go vet ./internal/portal/githttp/... ./internal/db/store/...` — passes
- `go test ./internal/portal/githttp/... ./internal/db/store/...` — all pass (SQLite path; no `JAMSESH_TEST_PG_DSN` set)
- Note: `go build ./...` has pre-existing failures in `internal/portal/tokens/` (`IssueAnonymousSessionBearer` missing from service_impl) from an in-progress sibling story in the same epic. These are unrelated to this story's changes.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `findBaseRefUpdate` (`internal/portal/githttp/receive_pack.go:382-386`) contains a dead `strings.Count(rest, "/") != 1` guard after an exact-string equality check on `u.Ref == want`. Since `want` already pins the exact two-segment shape and session IDs are ULIDs (no slashes), the count branch is unreachable. Author flags this in the comment ("Shouldn't happen given the exact-string match above"). Harmless; could be deleted in a future tidy-up.

**Notes**:
- Verified post-receive stamping placement: inserted immediately after the draft-ref seeding loop (line ~287) and before `Emitter.EmitForUpdates` (line ~310). Failure path is non-fatal (warn-log only), preserving the RPO=0 contract — the event-emission/object-storage sync stays the gating point for push success.
- Verified the re-stamp protection chain: (a) `findBaseRefUpdate` only matches `refs/heads/jam/<sessionID>/base`; (b) pre-receive (`internal/portal/prereceive/refs.go:104-109`) rejects any push to the base ref once refs exist. So a second push to base is impossible, and a user ref ending in `/base` like `refs/heads/jam/<id>/<accountID>/base` does not match. Correct by construction; SQL has no `WHERE base_sha IS NULL` guard but doesn't need one.
- Foundation docs unchanged: `docs/UX.md:63` describes the failure case (`base_sha: null` when push fails); the success case this story implements is fully consistent. No drift.
- Build/vet/test verified locally: `go build ./internal/portal/githttp/... ./internal/db/store/...` OK, `go vet` OK, the three new tests pass (SQLite path; PG path gated on `JAMSESH_TEST_PG_DSN`).
- Test design is sound: `passthroughStore` embedding override + `setBaseSHAFn` function field is a clean failure-injection seam, and the assertions cover both the warn log path (via `setBaseSHACalled`) and the durability path (ref present in bare repo after push).
