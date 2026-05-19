---
id: gate-patterns-v0.1.0
kind: story
stage: done
tags: [patterns]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: patterns
created: 2026-05-18
updated: 2026-05-18
---

# Patterns extracted for v0.1.0

## New patterns codified

- `authfail-three-branch-guard` — Strict-server handlers gate on
  `handlerauth.Require*`, branch on 500 vs typed 401/403 mapper vs
  happy path
- `deperr-translate-pipeline` — Three-tier error translation: wrap with
  `deperr.Wrap*`, classify via `errors.Is`, emit typed envelope via
  `httperr.Err*Unavailable`
- `tx-emit-then-fanout` — Mutate + allocate seq + insert event in one
  `store.WithTx`; fanout to WebSocket subscribers after commit
- `dual-dialect-mirror-queries` — Mirror SQL between
  `db/queries/sqlite/` and `db/queries/postgres/` with identical query
  names, columns, and `org_id` scoping
- `per-package-clock-interface` — Each package defines its own
  `Clock interface{Now() time.Time}` + `realClock{}` fallback;
  `*testclock.AdvanceableClock` advances all by structural typing
- `testcontainers-fixture-shape` — `tests/e2e/fixtures/<dep>/` packages
  with dual host-side and container-side address fields, plus
  `Start(ctx, t, Options) *<Type>` that registers `t.Cleanup`
- `snippet-children-component` — Reusable Svelte primitives take typed
  `children: Snippet` via `$props()` with string-literal-union
  variants; render via `{@render children()}`
- `openapi-fetch-middleware-client` — Single shared
  `client = createClient<paths>(...)` with `bearerMiddleware` +
  `unauthorizedMiddleware` attached via `client.use(...)`;
  `lateFetch` indirection enables vitest stubGlobal

## Inconsistencies flagged

None. This is the project's first pattern extraction; no pre-existing
catalog to diverge from.

## Pattern files written

- `.claude/skills/patterns/authfail-three-branch-guard.md`
- `.claude/skills/patterns/deperr-translate-pipeline.md`
- `.claude/skills/patterns/tx-emit-then-fanout.md`
- `.claude/skills/patterns/dual-dialect-mirror-queries.md`
- `.claude/skills/patterns/per-package-clock-interface.md`
- `.claude/skills/patterns/testcontainers-fixture-shape.md`
- `.claude/skills/patterns/snippet-children-component.md`
- `.claude/skills/patterns/openapi-fetch-middleware-client.md`
- `.claude/skills/patterns/SKILL.md` (catalog)
- `.claude/rules/patterns.md` (index)
