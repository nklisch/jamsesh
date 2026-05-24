---
id: story-adapter-wrap-helpers-step-1-define
kind: story
stage: review
tags: [portal, refactor]
parent: feature-refactor-adapter-generic-wrap-helpers
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-24
updated: 2026-05-24
---

# Define wrap1 + wrapList generic helpers in store package

## Brief

Step 1 of the parent feature: define the two Go-generic helpers that
collapse the mechanical wrapper boilerplate in
`internal/db/store/sqlite_adapter.go` and `postgres_adapter.go`. Pure
addition — no existing callers touched.

## Current state

Every scalar-return adapter method follows this 5-line shape:
```go
func (a *sqliteAdapter) GetOrgByID(ctx context.Context, id string) (Org, error) {
    row, err := a.q.GetOrgByID(ctx, id)
    if err != nil { return Org{}, mapSQLiteErr(err) }
    return sqliteOrg(row), nil
}
```

Every list-return follows a 7-line shape with an explicit conversion loop:
```go
func (a *sqliteAdapter) ListSessionsForOrg(...) ([]Session, error) {
    rows, err := a.q.ListSessionsForOrg(ctx, orgID)
    if err != nil { return nil, mapSQLiteErr(err) }
    sessions := make([]Session, len(rows))
    for i, r := range rows { sessions[i] = sqliteSession(r) }
    return sessions, nil
}
```

Action methods (returning `error` only) are already 1-3 lines —
they don't benefit from a wrap helper. **Skip wrapping action methods.**

## Target state

New file `internal/db/store/wrap.go` (package-private helpers):

```go
package store

// wrap1 wraps a one-row dialect query: returns convert(row) on success,
// or the zero value plus mapErr(err) on failure. Used to collapse the
// scalar-return wrapper methods in the dialect adapters from 5 lines
// to 1.
func wrap1[R any, D any](row R, err error, mapErr func(error) error, convert func(R) D) (D, error) {
    if err != nil {
        var zero D
        return zero, mapErr(err)
    }
    return convert(row), nil
}

// wrapList wraps a multi-row dialect query: returns a slice of
// convert(row) on success, or nil plus mapErr(err) on failure. Used
// to collapse the list-return wrapper methods from 7 lines to 1.
func wrapList[R any, D any](rows []R, err error, mapErr func(error) error, convert func(R) D) ([]D, error) {
    if err != nil {
        return nil, mapErr(err)
    }
    out := make([]D, len(rows))
    for i, r := range rows {
        out[i] = convert(r)
    }
    return out, nil
}
```

## Tests

Add `internal/db/store/wrap_test.go`:

- `TestWrap1_Success` — returns convert(row), nil.
- `TestWrap1_Error` — returns zero, mapErr(err).
- `TestWrapList_Success` — returns mapped slice, nil.
- `TestWrapList_Error` — returns nil, mapErr(err).
- `TestWrapList_EmptySlice` — returns `[]D{}` (or zero-length), nil.

Use trivial concrete types (e.g. `int` source, `string` destination) so the
tests are obvious.

## Acceptance criteria

- [ ] `internal/db/store/wrap.go` exports the two generic helpers
      (lowercase — package-private).
- [ ] `internal/db/store/wrap_test.go` covers all 5 cases above.
- [ ] No existing adapter methods touched in this story.
- [ ] `go build ./...` clean.
- [ ] `go test ./internal/db/store/...` clean.

## Risk

**Very low.** Pure addition; no callers in this story.

## Rollback

`git revert` the implementation commit.

## Notes

The helpers use Go generics (1.21+). The project already uses generics
elsewhere; verify by `grep -rn "^func [a-zA-Z]*\[" internal/` if uncertain.

## Implementation notes

- Created `internal/db/store/wrap.go` with the exact `wrap1` and `wrapList`
  signatures from the story body, plus a package-level comment block pointing
  at the parent feature.
- Created `internal/db/store/wrap_test.go` with all 5 required tests using
  `int` → `string` as the trivial type pair:
  - `TestWrap1_Success` — asserts `convert(row)` returned, nil error.
  - `TestWrap1_Error` — asserts zero value (`""`), `mapErr` invoked, error
    wraps sentinel via `errors.Is`.
  - `TestWrapList_Success` — asserts correctly mapped `[]string`, nil error.
  - `TestWrapList_Error` — asserts nil slice, `mapErr` invoked, error wraps
    sentinel.
  - `TestWrapList_EmptySlice` — asserts non-nil zero-length slice, nil error,
    `mapErr` not called.
- `go build ./...` clean; `go test ./...` clean (all packages pass).
- No existing adapter methods touched.
