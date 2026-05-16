---
id: epic-portal-ui-design-system
kind: feature
stage: drafting
tags: [ui]
parent: epic-portal-ui
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal UI — Design System

## Brief

Realizes the locked design system (Quiet Slate palette + Geist typography)
as actual Svelte 5 base components and a token-import pipeline. The mockup
phase already produced `tokens.css` with both light and dark mode and the
explicit `[data-theme]` toggle mechanism; this feature wires that file into
the Vite build and ships the small set of reusable components every other
portal-UI feature consumes.

Components to ship:

- `Button` (primary / ghost / accent variants matching the palette mocks)
- `Input` (text, email)
- `Card` (the surface container used across sessions and the artifact pane)
- `Badge` / `Pill` (mode pill — sync / isolated; conflict pill)
- `AuthorDot` (takes a user id, returns a colored dot using `--author-N`)
- `ThemeToggle` (cycles light / dark / system, sets `[data-theme]` on
  `<html>`, persists choice)
- `InlineCode` (matching the `<code>` styling in the mocks)

Independent of `foundation` because tokens + base components don't need
routing or auth to design and validate. The two features run in parallel
and the consuming features (session-list, session-view-shell, etc.) join
their output downstream.

Does NOT cover: any session-bound or auth-bound UI (those land in
consuming features).

## Epic context

- Parent epic: `epic-portal-ui`
- Position in epic: independent foundation feature — runs in parallel with
  `epic-portal-ui-foundation`. Every consuming feature depends on both.

## Foundation references

- `docs/SPEC.md` — Stack > Portal frontend
- `docs/UX.md` — Portal UI surfaces (component vocabulary)
- `.mockups/design-system/tokens.css` — the source of truth this feature
  realizes in Svelte
- `.mockups/design-system/palette.html` — visual reference for component
  states
- `.mockups/design-system/typography.html` — type scale + weight usage

<!-- Feature-design will fill in component APIs, slot/prop signatures, and
test approach when /agile-workflow:feature-design runs on this. -->
