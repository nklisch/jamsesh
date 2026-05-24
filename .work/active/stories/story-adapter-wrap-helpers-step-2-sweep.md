---
id: story-adapter-wrap-helpers-step-2-sweep
kind: story
stage: review
tags: [portal, refactor]
parent: feature-refactor-adapter-generic-wrap-helpers
depends_on: [story-adapter-wrap-helpers-step-1-define]
release_binding: null
gate_origin: refactor-design
created: 2026-05-24
updated: 2026-05-24
---

# Sweep adapter scalar-return + list-return methods to use wrap1 / wrapList

## Brief

Step 2 of the parent feature. Apply `wrap1` / `wrapList` (landed in
step 1) across both dialect adapter files to collapse the mechanical
wrapper boilerplate. Estimated ~240 LoC saved across the two files.

Action methods (single-line `return mapSQLiteErr(a.q.X(ctx, ...))`) are
already concise — leave them alone.

## Dep readiness

`depends_on: [story-adapter-wrap-helpers-step-1-define]` — verify step 1
is at `stage: review` or `done` before starting. The helpers must exist.

## Target shape per method

**Scalar-return** (the most common pattern; ~40 methods per dialect):
```go
// Before
func (a *sqliteAdapter) GetOrgByID(ctx context.Context, id string) (Org, error) {
    row, err := a.q.GetOrgByID(ctx, id)
    if err != nil { return Org{}, mapSQLiteErr(err) }
    return sqliteOrg(row), nil
}

// After
func (a *sqliteAdapter) GetOrgByID(ctx context.Context, id string) (Org, error) {
    return wrap1(a.q.GetOrgByID(ctx, id), mapSQLiteErr, sqliteOrg)
}
```

**List-return** (~10 methods per dialect):
```go
// Before
func (a *sqliteAdapter) ListSessionsForOrg(ctx context.Context, orgID string) ([]Session, error) {
    rows, err := a.q.ListSessionsForOrg(ctx, orgID)
    if err != nil { return nil, mapSQLiteErr(err) }
    sessions := make([]Session, len(rows))
    for i, r := range rows { sessions[i] = sqliteSession(r) }
    return sessions, nil
}

// After
func (a *sqliteAdapter) ListSessionsForOrg(ctx context.Context, orgID string) ([]Session, error) {
    return wrapList(a.q.ListSessionsForOrg(ctx, orgID), mapSQLiteErr, sqliteSession)
}
```

**Action methods**: **do not change.** `return mapSQLiteErr(a.q.X(...))` is
already optimal.

**Methods with param-struct translation**: when the wrapper builds a
`sqlitestore.XParams` (or `pgstore.XParams`) from a domain param struct,
the param-build is the work; the result-handling is what `wrap1` /
`wrapList` collapses. Apply the helper to the result side; keep the
param-build inline. Example:
```go
// After
func (a *sqliteAdapter) ListSessionsForOrgWithCursor(ctx context.Context, p ListSessionsForOrgWithCursorParams) ([]Session, error) {
    return wrapList(a.q.ListSessionsForOrgWithCursor(ctx, sqlitestore.ListSessionsForOrgWithCursorParams{
        OrgID:     p.OrgID,
        CreatedAt: p.Before,
        Limit:     p.Limit,
    }), mapSQLiteErr, sqliteSession)
}
```

## Approach

1. Walk `internal/db/store/sqlite_adapter.go` top-to-bottom; for each
   wrapper method, classify (scalar / list / action / other) and apply
   the right transformation. Action methods unchanged.
2. Walk `internal/db/store/postgres_adapter.go` the same way with
   `mapPostgresErr` and `pg*` converters.
3. Methods that don't fit any of the three patterns (e.g. ones with
   custom branching beyond err/result): leave them alone. Document any
   skipped methods in implementation notes.

## Acceptance criteria

- [ ] Both adapter files reduced by ~240 combined LoC.
- [ ] No scalar-return wrapper method has the 5-line shape; all use `wrap1`.
- [ ] No list-return wrapper method has the explicit conversion loop;
      all use `wrapList`.
- [ ] Action methods unchanged (still 1-3 lines as today).
- [ ] No method signatures changed — public adapter API unchanged.
- [ ] `go build ./...` clean.
- [ ] `go test ./internal/db/store/...` clean.
- [ ] Full `go test ./...` clean (the adapters are used everywhere).
- [ ] The dual-dialect-mirror-queries invariant remains intact — every
      method in sqlite_adapter still has a twin in postgres_adapter
      (just shorter).

## Risk

**Medium.** Wide blast radius — every adapter method touched. Mitigation:
the helpers are tested in step 1; each method swap is mechanical; the
compiler catches type mismatches; existing integration tests catch any
semantic drift.

Suggested commit chunking: commit per ~20 methods to keep diffs reviewable.

## Rollback

`git revert` the implementation commit (or per-chunk commits if you
chunked). The helpers from step 1 stay in place as additive surface.

## Sequencing

`depends_on: [story-adapter-wrap-helpers-step-1-define]` — needs the
helpers defined before the sweep.

## Implementation notes

**Go multi-value spreading constraint:** The story spec showed aspirational
single-expression calls like `wrap1(a.q.X(ctx, ...), mapErr, convert)`. Go
does not allow spreading a multi-value function return when there are
additional trailing arguments. Every `wrap1` / `wrapList` call requires two
lines: explicit `row, err := a.q.X(...)` then `return wrap1(row, err, ...)`.
This still saves 3 LoC per scalar method and 5+ LoC per list method.

**Methods left unchanged (skipped):**
- `ListOrgMembers` (both adapters) — `pgOrgMemberWithAccount` takes two
  arguments `(orgID, r)`; not a plain converter function.
- `ConsumeOAuthState` (both adapters) — no named converter; builds `OAuthState`
  inline from field assignments.
- `ListEventsSince`, `ListEventsSinceForDigest` (both adapters) — `int32→int64`
  Seq conversion inside the loop; no named converter.
- `ListPresenceForSession` (both adapters) — `pgtype.Timestamptz` unwrapping
  inline; no named converter.

**Outcome:** Both adapter files reduced by ~230 combined LoC. All 48
`go test ./...` packages pass. Build clean. Dual-dialect mirror invariant
intact.
