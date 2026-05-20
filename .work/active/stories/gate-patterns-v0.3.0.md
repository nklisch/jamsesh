---
id: gate-patterns-v0.3.0
kind: story
stage: done
tags: [patterns]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: patterns
created: 2026-05-20
updated: 2026-05-20
---

# Patterns extracted for v0.3.0

## New patterns codified

- `wrapper-object-rune-store` — Module-level rune stores in
  `frontend/src/lib/*.svelte.ts` keep `$state`/`$derived` in private
  `_`-prefixed vars and expose them via an `export const` plain-object
  facade with getter syntax. 3+ occurrences (`auth.svelte.ts`,
  `router.svelte.ts`, `ws.svelte.ts`).
- `openapi-fetch-result-branch` — Every `client.GET/POST/...` call
  destructures `{ data, error }` and branches on `if (data)` / `error`,
  reserving `try`/`catch` for transport throws. 4+ occurrences across
  screens.
- `spa-test-module-mock-barrel` — Each screen/component `*.test.ts`
  declares top-of-file `vi.mock(...)` blocks for every `$lib/`
  singleton, routing each verb through a module-scoped `const mockX =
  vi.fn()` via `(...args) => mockX(...args)` so the spy survives
  `vi.mock` hoisting. 3+ occurrences (`Home.test.ts`, `OrgSettings.test.ts`,
  `SessionList.test.ts`).
- `window-location-defineproperty-stub` — jsdom tests stub
  `window.location` via `Object.defineProperty(window, 'location',
  { value: { ...window.location, ... }, writable: true, configurable:
  true })`, often wrapped in a `setSearch` / `setHash` helper. 4+
  occurrences (`OAuthCallback.test.ts`, `MagicLinkExchange.test.ts`,
  `Login.test.ts`, `router.test.ts`).
- `view-state-union-machine` — Screens drive UI state with a
  string-literal-union type + single `$state<ViewState>(...)` rune +
  `{#if viewState === 'a'}` template branching. 5+ occurrences
  (`OAuthCallback.svelte`, `InviteAccept.svelte`, `Login.svelte`,
  `Home.svelte`, `MagicLinkExchange.svelte`).
- `same-origin-returnto-guard` — User-supplied `return_to` strings are
  validated with `returnTo && returnTo.startsWith('/') &&
  !returnTo.startsWith('//')` before passing to `navigate(...)`. 3
  occurrences (`Login.svelte`, `OAuthCallback.svelte`,
  `MagicLinkExchange.svelte`).

## Inconsistencies flagged

None. The bundle's `Input.svelte` is consistent with
`snippet-children-component` (leaf input wrapper, no children prop
appropriate), and all API calls use the shared `client` from
`openapi-fetch-middleware-client`.

## Pattern files written

- `.claude/skills/patterns/wrapper-object-rune-store.md`
- `.claude/skills/patterns/openapi-fetch-result-branch.md`
- `.claude/skills/patterns/spa-test-module-mock-barrel.md`
- `.claude/skills/patterns/window-location-defineproperty-stub.md`
- `.claude/skills/patterns/view-state-union-machine.md`
- `.claude/skills/patterns/same-origin-returnto-guard.md`
- `.claude/rules/patterns.md` (index updated)
- `.claude/skills/patterns/SKILL.md` (available patterns list updated)
