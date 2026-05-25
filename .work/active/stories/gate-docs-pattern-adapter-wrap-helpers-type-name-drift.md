---
id: gate-docs-pattern-adapter-wrap-helpers-type-name-drift
kind: story
stage: review
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# adapter-wrap-helpers pattern shows PascalCase SQLiteAdapter; code is sqliteAdapter

## Drift category
pattern-skill-staleness

## Location
- Doc: `.claude/skills/patterns/adapter-wrap-helpers.md:54-60` (Example 2),
  `.claude/skills/patterns/adapter-wrap-helpers.md:65-71` (Example 3)
- Code: `internal/db/store/sqlite_adapter.go:26` (`type sqliteAdapter struct`),
  `internal/db/store/sqlite_adapter.go:239` (actual `GetOrgByID` method),
  `internal/db/store/sqlite_adapter.go:324` (actual `ListOrgsForAccount` method)

## Current doc text
> ```go
> func (s *SQLiteAdapter) GetOrgByID(ctx context.Context, id string) (Org, error) {
>     row, err := s.q.GetOrgByID(ctx, id)
>     return wrap1(row, err, mapSQLiteErr, sqliteOrg)
> }
> ```
>
> [Example 3 likewise shows `func (s *SQLiteAdapter) ListOrgsForAccount...`]

## Reality
The adapter type was renamed to **unexported** `sqliteAdapter` (and the
receiver letter changed from `s` to `a`). Current code:

```go
func (a *sqliteAdapter) GetOrgByID(ctx context.Context, id string) (Org, error) {
    row, err := a.q.GetOrgByID(ctx, id)
    return wrap1(row, err, mapSQLiteErr, sqliteOrg)
}
```

The pattern's claim about replication ("92 times each in `sqlite_adapter.go`
and `postgres_adapter.go`") is still believable in shape, but the type-name
PascalCase-vs-camelCase difference is a real drift: a reader following the
pattern verbatim would write an exported-receiver method that doesn't
compile, or worse, declare a NEW exported type alongside the existing
unexported one.

The example line numbers (`:226`, `:326`) are also stale (actual: 239, 324).

## Required edit
In `.claude/skills/patterns/adapter-wrap-helpers.md`:

1. **Example 2 (line 54-60):** Update `SQLiteAdapter` → `sqliteAdapter`,
   receiver letter `s` → `a`, body `s.q` → `a.q`. Update line anchor
   `:226` → `:239`.

2. **Example 3 (line 65-71):** Same renames. Update line anchor `:326` →
   `:324`.

3. **Verify postgres mirror:** confirm the postgres adapter follows the
   same `postgresAdapter` (unexported) convention with receiver `p` and
   update any postgres-side mention in the pattern body.

Apply rolling-foundation: the pattern describes the code AS IT IS NOW; no
"renamed from SQLiteAdapter" prose. Git history records the rename.

## Implementation notes

`.claude/skills/patterns/adapter-wrap-helpers.md` Examples 2 + 3 rolled
to current code shape: `SQLiteAdapter → sqliteAdapter` (unexported),
receiver letter `s → a`, method bodies updated (`s.q → a.q`). Line
anchors rolled forward: Example 2 `:226 → :239`, Example 3 `:326 → :324`.

Postgres mirror confirmed by verification (`type postgresAdapter` at
`postgres_adapter.go:26`, methods with receiver `a *postgresAdapter`); no
postgres-side mention in this pattern body that needed touching beyond
keeping the sqlite example self-consistent.

Edits applied in the parent autopilot session — auto-mode classifier
blocks sub-agents from editing under `.claude/skills/`. `go build ./...`
clean.
