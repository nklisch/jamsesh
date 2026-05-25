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

### Initial run (2026-05-24)

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

### Re-run extension (2026-05-24, post `store.Store`-narrowing refactor)

- `package-private-composed-store-interface` — Portal consumer packages
  declare a lowercase `<pkg>Store interface` composed from `store.*`
  sub-interfaces (plus `WithTx`) and accept it in their constructor;
  `cmd/portal/main.go` passes the full adapter and structural typing
  matches each narrow interface. 20 production occurrences across
  accounts, automerger×2, auth×3, comments, events, finalize, githttp,
  handlerauth×2, mcpendpoint, playground×3, sessions, storage, tokens,
  wsgateway.
- `test-narrow-store-delegation` — Test files inject typed store
  failures via `failing<Verb><Entity>Store` wrappers that delegate every
  consumer-interface method to a real store except the single
  method-under-test; never `struct { store.Store }` embedding.
  6 wrappers across 5 files (accounts ×2, sessions, comments ×2,
  finalize).
- `testenv-harness-struct` — Each portal package's tests bundle wired
  deps (`*httptest.Server`, real `store.Store`, tokens service, stubs)
  in an unexported `testEnv struct` constructed via
  `newTestEnv(t *testing.T) *testEnv` with optional
  `WithStore`/`WithClock`/`WithTokens` overloads. 8 packages: tokens,
  githttp, sessions, wsgateway, comments, mcpendpoint, playground,
  testclock.
- `reserved-org-id-local-const-mirror` — Cross-cutting reserved
  identifiers (`playgroundOrgID = "org_playground"`) are mirrored as
  lowercase package-local `const` in each consumer with a comment
  pinning them to `playground.ReservedOrgID` to break import cycles
  without widening the dependency graph. 4 production occurrences
  (sessions, comments, githttp, prereceive) plus 3 test-scope inline
  mirrors.

## Inconsistencies flagged

None. Both the original 5 and the re-run extension's 4 are additive:
they extend the existing pattern catalog with new variants but do not
violate any documented pattern.

## Pattern files written

Initial run:

- `.claude/skills/patterns/per-instance-factory-rune-store.md`
- `.claude/skills/patterns/adapter-wrap-helpers.md`
- `.claude/skills/patterns/strict-server-partial-handler-shim.md`
- `.claude/skills/patterns/playground-activity-reset.md`
- `.claude/skills/patterns/ticker-sweep-loop.md`

Re-run extension:

- `.claude/skills/patterns/package-private-composed-store-interface.md`
- `.claude/skills/patterns/test-narrow-store-delegation.md`
- `.claude/skills/patterns/testenv-harness-struct.md`
- `.claude/skills/patterns/reserved-org-id-local-const-mirror.md`

Combined updates:

- `.claude/skills/patterns/SKILL.md` (updated available-patterns list)
- `.claude/rules/patterns.md` (updated index — 25 entries, under cap)

## Discovery summary

- Initial run: 238 bundle files + ~60 immediate consumers scanned;
  12 candidates evaluated; 5 genuine patterns (3+ occurrences);
  0 inconsistencies.
- Re-run extension: 190 incremental code files (since 2026-05-24) +
  20+ consumer packages re-scanned; 9 candidates evaluated; 4 genuine
  new patterns; 0 inconsistencies. Candidates rejected for <3
  occurrences: slog JSON-handler capture (2 files), barrier+WaitGroup
  race orchestration (2 files), table-driven boundary tests (stdlib
  Go idiom, not project-specific).
- Total v0.4.0 patterns codified: 9.
