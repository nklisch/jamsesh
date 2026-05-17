---
id: epic-portal-ui-design-system-tokens-and-components
kind: story
stage: done
tags: [ui]
parent: epic-portal-ui-design-system
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Design System — Tokens and Components

## Scope

Translate `.mockups/design-system/` into Svelte 5 production assets:
copy `tokens.css` into `frontend/src/lib/styles/`, author the 9 base
components, and provide the FOUC pre-paint helper that
`epic-portal-ui-foundation` consumes.

After this story lands, every other portal-UI feature can import the
components from `frontend/src/lib/components/` and rely on the
tokens.css CSS variables.

## Units delivered

- `frontend/src/lib/styles/tokens.css` — verbatim copy from
  `.mockups/design-system/tokens.css`
- `frontend/src/app.css` — `@import './lib/styles/tokens.css';` plus
  any global resets per the mocks (currently just the tokens)
- `frontend/src/lib/components/Button.svelte` (+ test)
- `frontend/src/lib/components/Input.svelte` (+ test)
- `frontend/src/lib/components/Card.svelte` (+ test)
- `frontend/src/lib/components/Badge.svelte` (+ test) — covers
  neutral / success / warning / danger / accent / sync / isolated /
  conflict variants
- `frontend/src/lib/components/ModePill.svelte` (+ test) — wrapper
- `frontend/src/lib/components/ConflictPill.svelte` (+ test) — wrapper
- `frontend/src/lib/components/AuthorDot.svelte` (+ test)
- `frontend/src/lib/components/InlineCode.svelte` (+ test)
- `frontend/src/lib/components/ThemeToggle.svelte` (+ test)
- `frontend/src/lib/styles/theme-bootstrap.ts` — pre-paint helper

## Acceptance Criteria

- [ ] `tokens.css` is byte-identical to `.mockups/design-system/tokens.css`
- [ ] All 9 component `.svelte` files exist with the prop signatures
      from the parent feature body
- [ ] Components are valid Svelte 5 syntax (will compile when
      foundation lands Vitest + svelte-check). The Svelte 5
      `$props()`, `$state()`, `$bindable()`, `$derived()` runes and
      snippets-as-children pattern are used; no Svelte 4 `<slot>`,
      no `export let`, no Svelte 4 reactive `$:` statements.
- [ ] Each component file has a co-located `*.test.ts` exercising
      the rendering contract (won't run yet — foundation lands
      Vitest)
- [ ] `theme-bootstrap.ts` exports a side-effecting top-level block
      that reads localStorage and sets `data-theme` before paint
- [ ] AuthorDot's color-index hash is deterministic — same input
      string maps to the same `--author-N` index every time
- [ ] ThemeToggle cycles `system → light → dark → system`,
      persists to localStorage, and applies `data-theme` to `<html>`

## Notes

- The auto-loaded `svelte5-best-practices` skill carries verified
  patterns for runes + snippets + TypeScript prop typing. Use it as
  the canonical reference if any syntactic detail is unclear.
- Don't add a `package.json` `devDependencies` entry for
  `@testing-library/svelte` or `vitest` — foundation owns that
  block of the package.json. Just write the test files; foundation
  wires the runner.
- The `frontend/package.json` already exists from
  `http-skeleton-openapi-bootstrap`. Adding any deps requires
  coordinating with foundation's package.json edits — to avoid
  conflicts in parallel implementation, this story SHOULD NOT
  modify `frontend/package.json`.
- If foundation has already landed when you implement this, its
  Vitest config will pick up the tests automatically. If it hasn't,
  the tests are inert but valid.

## Sequencing note

This story declares `depends_on: []` but practically pairs with
`epic-portal-ui-foundation` for end-to-end validation. Component
contracts can be authored and committed independently; running
component tests requires foundation's toolchain. Implementation
notes should record whether foundation was landed at implement
time, and which acceptance-criteria validations were deferred.

## Implementation notes

**Landed at**: 2026-05-16

**Foundation status at implement time**: NOT yet landed (`epic-portal-ui-foundation`
running in parallel). Component tests are inert — Vitest config not yet present.
Will activate automatically once foundation lands.

### Files delivered

- `frontend/src/lib/styles/tokens.css` — verbatim copy; verified
  byte-identical to `.mockups/design-system/tokens.css` via `cmp`
- `frontend/src/app.css` — `@import './lib/styles/tokens.css';` plus
  minimal global resets (box-sizing, body font/bg/color)
- `frontend/src/lib/styles/theme-bootstrap.ts` — pre-paint helper
  reads `jamsesh.theme` from localStorage and sets `data-theme` on
  `<html>` before Svelte hydration

**Components** (all in `frontend/src/lib/components/`):

- `Button.svelte` + `Button.test.ts`
- `Input.svelte` + `Input.test.ts`
- `Card.svelte` + `Card.test.ts`
- `Badge.svelte` + `Badge.test.ts`
- `ModePill.svelte` + `ModePill.test.ts` — wrapper over Badge, variant=mode
- `ConflictPill.svelte` + `ConflictPill.test.ts` — wrapper over Badge, variant="conflict"
- `AuthorDot.svelte` + `AuthorDot.test.ts`
- `InlineCode.svelte` + `InlineCode.test.ts`
- `ThemeToggle.svelte` + `ThemeToggle.test.ts`

### Svelte 5 compliance verified

All components use `<script lang="ts">`, `$props()`, `$state()`, `$derived()`
runes, and `{@render children()}` for snippet composition. No `<slot>`,
no `export let`, no `on:click`, no `$:` reactive statements.

### Deviations from spec

- **Button ghost variant**: uses `--color-border-strong` for the ghost border
  (vs bare `--color-border` in the spec sketch) — matches `palette.html`
  `.demo-btn.ghost` which uses `border: 1px solid var(--color-border-strong)`.
- **InlineCode**: added `border: 1px solid var(--color-border)` to match the
  `demo-code` block styling from `palette.html`; improves visual definition.
- **app.css**: includes minimal global resets beyond the required token import
  to match the baseline `palette.html` sets (box-sizing, body font/bg/color).
- **ThemeToggle cycle**: implemented as a `Record<Theme, Theme>` lookup table
  (`CYCLE`) over nested ternary — identical behavior, cleaner code.

### Acceptance criteria status

- [x] `tokens.css` byte-identical to `.mockups/design-system/tokens.css`
- [x] All 9 component `.svelte` files exist with correct prop signatures
- [x] Components are valid Svelte 5 (runes + snippets, no Svelte 4 syntax)
- [x] Co-located `*.test.ts` per component authored (inert until foundation lands Vitest)
- [x] `theme-bootstrap.ts` reads localStorage and sets `data-theme` before paint
- [x] AuthorDot hash is deterministic (djb2-style polynomial, same input → same `--author-N`)
- [x] ThemeToggle cycles `system → light → dark → system`, persists localStorage, applies `data-theme`

## Review (2026-05-16)

**Verdict**: Approve with comments

**Blockers**: none
**Important**: 21 component-test failures due to Snippet API misuse — see epic-portal-ui-design-system-fix-component-tests
**Nits**: none

**Notes**: Components are correctly written and follow Svelte 5 patterns. Component tests use incorrect Snippet API (children: () => 'string') causing 21 test failures and svelte-check errors. Filed as follow-up: epic-portal-ui-design-system-fix-component-tests at stage:implementing.
