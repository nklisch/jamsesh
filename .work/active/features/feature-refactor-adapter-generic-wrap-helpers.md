---
id: feature-refactor-adapter-generic-wrap-helpers
kind: feature
stage: implementing
tags: [portal, refactor]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Collapse adapter wrapper boilerplate via Go generics

## Brief

`internal/db/store/sqlite_adapter.go` (2301 LoC) and
`postgres_adapter.go` (2299 LoC) contain ~211 wrapper methods (~106
each) that follow a small set of mechanical shapes. Every method
looks like one of these forms, differing only in type bindings:

```go
// Scalar return
func (a *sqliteAdapter) GetSessionByID(ctx context.Context, id string) (Session, error) {
    row, err := a.q.GetSessionByID(ctx, id)
    if err != nil { return Session{}, mapSQLiteErr(err) }
    return sqliteSession(row), nil
}

// List return
func (a *sqliteAdapter) ListSessionsForOrg(ctx context.Context, orgID string) ([]Session, error) {
    rows, err := a.q.ListSessionsForOrg(ctx, orgID)
    if err != nil { return nil, mapSQLiteErr(err) }
    out := make([]Session, len(rows))
    for i, r := range rows { out[i] = sqliteSession(r) }
    return out, nil
}

// Action (no return body)
func (a *sqliteAdapter) DeleteSession(ctx context.Context, id string) error {
    return mapSQLiteErr(a.q.DeleteSession(ctx, id))
}
```

The dialect difference is purely type-bound: queries-package type
(`sqlitestore.Queries` vs `pgstore.Queries`), error mapper
(`mapSQLiteErr` vs `mapPostgresErr`), row converter (`sqliteSession`
vs `pgSession`).

Go 1.21+ generics handle this cleanly. Extract a small set of
`wrap1[R, D]`, `wrapList[R, D]`, `wrapAction` helpers; sweep the
adapter files; each method drops from 5 lines to 1-2.

## Design questions for refactor-design

The per-feature design pass should answer:

1. **Call-site survey.** Beyond the three obvious shapes above, are
   there other recurring shapes? Likely candidates:
   - Action with returning ID (`InsertX(...) (string, error)`)
   - Action returning a domain type (`UpdateX(...) (X, error)`)
   - Tx-wrapped variants (called inside `WithTx`)
   - Scalar non-domain return (`CountX(...) (int64, error)`)
   Survey the adapter files end-to-end and enumerate every shape.
   Each shape gets its own helper.

2. **Generic helper signatures.** A first sketch:
   ```go
   // wrap1: scalar return from a one-row query.
   func wrap1[R any, D any](row R, err error, mapErr func(error) error, convert func(R) D) (D, error) {
       if err != nil { var zero D; return zero, mapErr(err) }
       return convert(row), nil
   }

   // wrapList: list return from a multi-row query.
   func wrapList[R any, D any](rows []R, err error, mapErr func(error) error, convert func(R) D) ([]D, error) {
       if err != nil { return nil, mapErr(err) }
       out := make([]D, len(rows))
       for i, r := range rows { out[i] = convert(r) }
       return out, nil
   }

   // wrapAction: error-only.
   func wrapAction(err error, mapErr func(error) error) error {
       return mapErr(err)
   }
   ```
   Confirm these against the actual call sites; the design pass may
   refine signatures (e.g. accept context+func vs row+err depending
   on call-site ergonomics).

3. **Where do helpers live?** A new file
   `internal/db/store/wrap.go` is the obvious choice — sibling to
   `nullable_converters.go` from the prior dedup story.

4. **Order of landing.** Two strategies:
   - **One sweep per shape**: land `wrap1` + all scalar-return
     methods first; then `wrapList` + all list-return; etc.
     Cleaner commits, each one is mechanical and reviewable.
   - **One adapter at a time**: sweep all of `sqlite_adapter.go`
     first, then `postgres_adapter.go`. Bigger commits, harder
     review.
   The per-shape strategy is the better default; refactor-design
   should confirm.

5. **Row-converter dedup**. Is there a clean way to also generic-fy
   the row converters (`sqliteSession`, `pgSession`)? They're
   structurally similar but each touches dialect-specific field
   types. Likely NOT worth genericizing — each converter is a
   distinct mapping and they're already small. The design pass
   should explicitly disclaim this as out-of-scope unless an
   obvious win surfaces.

## Acceptance criteria (target)

- New `internal/db/store/wrap.go` exports the generic helpers
  (package-private; lowercase names).
- Both adapter files swept; every method that fits a helper shape
  uses it.
- Combined LoC of the two adapter files reduced by 600-900 LoC
  (~15-20%).
- No signature changes on the `Store` interface or any method.
- `go build ./...` and `go test ./...` clean across all 57 packages.
- The `dual-dialect-mirror-queries` pattern remains
  conventionally respected — adapters are still parallel mirrors,
  just shorter.

## Out of scope

- **Code-gen for the wrappers (Option B from the research pass).**
  Evaluated separately and deferred. Revisit conditions: a third
  dialect added, query count past ~150, schema overhaul, or merge
  conflicts in the adapter files become a pain point. See
  `feature-refactor-adapter-dialect-dedup` in the archive for the
  full deferral rationale.
- **Row-converter genericization** — not worth the loss of
  dialect-quirk readability.
- **`Store` interface partition** — covered by the sibling feature
  `feature-refactor-store-narrow-handler-signatures` (Layer 2),
  which lands AFTER this one for clean diff sequencing.

## Notes

Pure refactor. Tagged `[refactor]` so the design pass routes through
`refactor-design` per-feature mode. The research that evaluated the
heavier code-gen alternative concluded Option A (this feature) is
the right slice: meaningful LoC reduction without taking on a
generator-maintenance burden.

## Refactor Overview & Design (2026-05-24, autopilot)

Shape survey of `sqlite_adapter.go` (106 methods):
- ~40 scalar-return methods `(D, error)` — the 5-line `if err != nil ...` shape
- ~10 list-return methods `([]D, error)` — the 7-line explicit conversion-loop shape
- ~50 action methods `error` — already 1-3 lines; **wrap helpers add no value here**
- ~6 methods with custom branching — leave alone

Estimated LoC savings: ~240 across both files (down from the original
600-900 target — action methods are already concise enough that
`wrapAction` doesn't pay).

## Refactor Steps

### Step 1: Define wrap1 + wrapList helpers
**Priority**: High  **Risk**: Very Low
**Files**: `internal/db/store/wrap.go` (new), `internal/db/store/wrap_test.go` (new)
**Story**: `story-adapter-wrap-helpers-step-1-define`

Pure addition — 2 generic helpers + 5-test coverage. No existing callers touched.

### Step 2: Sweep both adapter files
**Priority**: High  **Risk**: Medium
**Files**: `internal/db/store/sqlite_adapter.go`, `internal/db/store/postgres_adapter.go`
**Story**: `story-adapter-wrap-helpers-step-2-sweep`
**Depends on**: Step 1

Apply `wrap1` to scalar-return methods, `wrapList` to list-return methods.
Action methods unchanged. Public adapter API unchanged. ~240 LoC saved.

## Implementation Order

Serial chain: Step 1 → Step 2. Step 1 lands the helpers (and tests them);
Step 2 applies them across the adapters.

## Design Decisions (autopilot)

- **No `wrapAction`** — action methods are already concise; a helper would
  add no value and dilute the pattern.
- **Param-struct translation stays inline** — the helpers wrap the result
  side; the param-build remains visible at the call site. This preserves
  dialect-quirk readability (the `dual-dialect-mirror-queries` pattern is
  about params + queries, which stay parallel; the helpers shorten only
  the result-handling).
- **Skip row-converter genericization** — confirmed out-of-scope by the
  earlier research pass. Each converter handles dialect-specific field
  types; genericizing would lose readability.
