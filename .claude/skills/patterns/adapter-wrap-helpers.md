# Adapter Wrap Helpers for Single-Row / List Queries

Dialect adapters in `internal/db/store/{sqlite,postgres}_adapter.go` collapse
the mechanical 5-line scalar-return and 7-line list-return wrappers around a
sqlc-generated call into a single line by calling the generic helpers
`wrap1[R,D]` / `wrapList[R,D]` defined in `internal/db/store/wrap.go`. Each
helper takes the raw `(row, err)` or `(rows, err)` plus the per-dialect
`mapErr` function and a `convert` function that converts the dialect row
type to the domain type.

## Rationale

Without the helpers, every adapter method that wraps one sqlc query
duplicates the same `if err != nil { return zero, mapErr(err) } ... return
convert(row), nil` shape. The dialect-mirror discipline (one query per
dialect, paired by name) means every such method is duplicated twice — once
in `sqlite_adapter.go`, once in `postgres_adapter.go`. With ~90 wrapped
methods per dialect, that's ~1,260 lines of boilerplate (~7 lines × 180
sites) collapsed into 180 single-expression returns. `wrap1` for scalars,
`wrapList` for slices. Both helpers are package-private — they're an
internal refactoring tool, not API.

## Examples

### Example 1: helper definition

**File**: `internal/db/store/wrap.go:15`

```go
// wrap1 wraps a one-row dialect query: returns convert(row) on success,
// or the zero value plus mapErr(err) on failure.
func wrap1[R any, D any](row R, err error, mapErr func(error) error, convert func(R) D) (D, error) {
    if err != nil {
        var zero D
        return zero, mapErr(err)
    }
    return convert(row), nil
}

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

### Example 2: scalar-return adapter method (sqlite)

**File**: `internal/db/store/sqlite_adapter.go:239`

```go
func (a *sqliteAdapter) GetOrgByID(ctx context.Context, id string) (Org, error) {
    row, err := a.q.GetOrgByID(ctx, id)
    return wrap1(row, err, mapSQLiteErr, sqliteOrg)
}
```

### Example 3: list-return adapter method (sqlite)

**File**: `internal/db/store/sqlite_adapter.go:324`

```go
func (a *sqliteAdapter) ListOrgsForAccount(ctx context.Context, accountID string) ([]Org, error) {
    rows, err := a.q.ListOrgsForAccount(ctx, accountID)
    return wrapList(rows, err, mapSQLiteErr, sqliteOrg)
}
```

Replicated 92 times each in `sqlite_adapter.go` and `postgres_adapter.go`
(184 total call sites). The convert function is always a per-row type
constructor named `<dialect><Type>` (e.g. `sqliteOrg`, `sqliteAccount`,
`sqliteSession`, `pgOrg`, `pgAccount`...).

## When to Use

- Adding a new method to one of the dialect adapters that wraps exactly one
  sqlc query and returns either one domain object or a slice of domain
  objects.
- The conversion can be expressed as a single function `func(R) D` with no
  extra context.

## When NOT to Use

- The adapter method composes multiple sqlc calls inside a `WithTx` — that's
  the `tx-emit-then-fanout` shape; the body has too many steps for
  `wrap1`/`wrapList` to apply.
- The conversion needs additional inputs not present in the row (e.g. an
  in-memory join, a derived field computed from other data). Inline the
  explicit `if err != nil` instead.
- The error mapping is anything other than `mapSQLiteErr` / `mapPostgresErr`
  — the helpers presume the single per-dialect mapper.

## Common Violations

- Re-inlining the 5-line wrapper after the helpers were introduced. The
  whole point of `feature-refactor-adapter-generic-wrap-helpers` was to
  eliminate that boilerplate; new methods should use the helpers from the
  start.
- Defining a new `mapErr` ad-hoc per call rather than reusing the
  package-level `mapSQLiteErr` / `mapPostgresErr`.
- Using `wrapList` for a method that returns nil on empty (the helper
  allocates a zero-length slice via `make([]D, len(rows))` — semantically
  equivalent but a different identity, which can matter in tests).
