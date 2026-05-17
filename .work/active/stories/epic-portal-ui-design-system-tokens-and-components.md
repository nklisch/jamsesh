---
id: epic-portal-ui-design-system-tokens-and-components
kind: story
stage: implementing
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
