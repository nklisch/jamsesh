---
id: gate-patterns-v0.4.0
kind: story
stage: done
tags: [patterns]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: patterns
created: 2026-05-24
updated: 2026-05-24
---

# Patterns extracted for v0.4.0

## New patterns codified

- `per-instance-factory-rune-store` — Rune stores needing per-mount isolation
  use `export function create<Name>(...)` returning a plain-object facade
  with `$state`/`$derived` in the closure body. 5 factories in v0.4.0 in
  `frontend/src/lib/{session,components}/`.
- `adapter-wrap-helpers` — Single-row/list adapter methods in
  `internal/db/store/{sqlite,postgres}_adapter.go` collapse to one line via
  package-private generics `wrap1[R,D]` / `wrapList[R,D]` in `wrap.go`. 184
  call sites across both dialects.
- `strict-server-partial-handler-shim` — Per-package handler test files
  define a `<pkg>OnlyStrict struct { *<pkg>.Handler }` wrapper whose
  receiver methods `panic("not wired")` for every `StrictServerInterface`
  operation owned by another package. 8 packages, 244 panic-stubs.
- `playground-activity-reset` — Substantive-action write surfaces on
  playground sessions call `store.ResetSessionIdleTimer` after the primary
  mutation succeeds, guarded by `orgID == playgroundOrgID &&
  IdleTimeout > 0`; failure is `slog.WarnContext`-logged and swallowed. 3
  surfaces: comments, finalize, post-receive.
- `ticker-sweep-loop` — Background workers use
  `ticker := time.NewTicker(d); defer ticker.Stop(); for { select { case <-<stop>: return; case <-ticker.C: doOnePass() } }`.
  5 workers: playground destruction sweep, objectstore lifecycle,
  ws-gateway ticket janitor, lease retention, lease heartbeat.

## Inconsistencies flagged

None. The five new shapes are additive: they extend the existing pattern
catalog with new variants but do not violate any documented pattern.

## Pattern files written

- `.claude/skills/patterns/per-instance-factory-rune-store.md`
- `.claude/skills/patterns/adapter-wrap-helpers.md`
- `.claude/skills/patterns/strict-server-partial-handler-shim.md`
- `.claude/skills/patterns/playground-activity-reset.md`
- `.claude/skills/patterns/ticker-sweep-loop.md`
- `.claude/skills/patterns/SKILL.md` (updated available-patterns list)
- `.claude/rules/patterns.md` (updated index)

## Discovery summary

- Files scanned: 238 bundle files + ~60 immediate consumers
- Pattern candidates evaluated: 12
- Genuine patterns (3+ occurrences): 5
- Inconsistencies with existing patterns: 0
