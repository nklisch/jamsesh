---
id: story-portal-session-attach-onboarding-help-link
kind: story
stage: review
tags: [ui]
parent: feature-portal-session-attach-onboarding
depends_on: [story-portal-session-attach-onboarding-walkthrough-component]
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-21
---

# AttachHelpLink chrome trigger

Implements `frontend/src/lib/components/AttachHelpLink.svelte` — a small
chrome-friendly trigger that wraps `<SessionAttachWalkthrough>` and exposes
a single click target ("Setup help" / "?") for portal chrome slots.

The full design is in the parent feature body under
`## Implementation Units → Unit 2`. Read that for props contract,
variant behavior, and acceptance criteria.

## Summary

- Props: `{ sessionId: string | null, variant?: 'inline' | 'icon' }`
- `'inline'` (default): text link reading "Setup help" with `?` prefix
- `'icon'`: 28×28 ghost button containing just the `?` glyph
- Internally owns `let open = $state(false)` and conditionally renders
  the walkthrough
- Forwards `sessionId` unchanged

## Test file

`frontend/src/lib/components/AttachHelpLink.test.ts` (new).

Setup: `vi.mock('./SessionAttachWalkthrough.svelte', ...)` to test the
link in isolation. Spy on the rendered walkthrough's `open` prop.

## Negative-case discipline

Same pattern as the component story — mutate, verify test fails, restore.

## Implementation notes

### Files created

- `frontend/src/lib/components/AttachHelpLink.svelte` — the component
- `frontend/src/lib/components/AttachHelpLink.test.ts` — 10 tests

### Variant approach

- `'inline'` (default): `<button class="help-btn help-btn--inline">` with a `<span
  class="help-glyph">?</span>` prefix glyph and "Setup help" label text. Ghost
  button styling mirrors `.signout-btn` in `Home.svelte` but uses
  `var(--font-size-xs)` and `var(--font-weight-medium)` for a more subtle feel.
- `'icon'`: same ghost button, `width/height: 28px; padding: 0`, square — just
  the `?` glyph. `aria-label="Setup help"` for accessibility.

Both variants share a `.help-btn` base class. Hover uses
`var(--color-bg-tertiary)` background + `var(--color-border-strong)` border.

### Walkthrough render strategy

Unconditional render of `<SessionAttachWalkthrough>` (always in the tree).
The walkthrough itself short-circuits to nothing when `open === false`, so
there is no DOM overhead. Props: `open={open}`, `sessionId={sessionId}`,
`onclose={() => (open = false)}`. No `onopenSession` prop — defaults to
`onclose` inside the walkthrough.

### Test strategy: Approach 1 (integration, no walkthrough mock)

Tests render `AttachHelpLink` against the real `SessionAttachWalkthrough`.
Coverage:

1. Inline button renders by default — `getByRole('button', { name: /setup help/i })`
2. Explicit `variant="inline"` renders correctly
3. No dialog in DOM before click — `querySelector('[role="dialog"]')` returns null
4. Click inline button → dialog appears in DOM
5. ESC key → dialog removed from DOM
6. Backdrop click → dialog removed from DOM
7. Icon variant renders (aria-label present, no visible "Setup help" text)
8. Click icon button → dialog appears
9. `sessionId` forwarded — session id found in rendered walkthrough (eyebrow + cc-cmd)
10. `sessionId=null` — chrome-help placeholder text renders; "Claude Code setup" eyebrow

### Negative-case verification

Temporarily commented out `open = true` in both click handlers one at a time:

- **Inline handler muted**: tests 4, 5, 6, 9, 10 all failed (5/10 failures). Test 4
  ("clicking the inline link opens the walkthrough dialog") correctly failed.
- **Icon handler muted**: test 8 ("clicking the icon button opens the walkthrough
  dialog") correctly failed.

Restored both handlers; all 10 tests pass. Full suite: 511 tests, 44 files, 0
failures, 0 regressions.
