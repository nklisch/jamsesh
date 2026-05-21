---
id: story-portal-session-attach-onboarding-sessionviewshell-affordance
kind: story
stage: done
tags: [ui]
parent: feature-portal-session-attach-onboarding
depends_on: [story-portal-session-attach-onboarding-help-link]
release_binding: v0.3.1
gate_origin: null
created: 2026-05-20
updated: 2026-05-21
---

# SessionViewShell chrome affordance

Add `<AttachHelpLink sessionId={sessionId} />` to the
`frontend/src/lib/screens/SessionViewShell.svelte` chrome, alongside the
existing `ThemeToggle`.

The full design is in the parent feature body under
`## Implementation Units → Unit 5`. The unit is small — one import,
one element in the chrome, no state.

## Summary

- Import `AttachHelpLink` from `$lib/components/AttachHelpLink.svelte`
- Add `<AttachHelpLink sessionId={sessionId} />` in the SessionViewShell
  header, near the ThemeToggle
- `sessionId` is the existing prop the component already receives

## Test file

`frontend/src/lib/screens/SessionViewShell.test.ts` (modify existing).

New tests:
- AttachHelpLink renders in the SessionViewShell chrome
- The forwarded `sessionId` matches the route param

Existing tests must still pass.

## Negative-case discipline

Verify by removing the new component from the chrome — the new render
test should fail. Restore.

## Implementation notes

**Placement.** `<AttachHelpLink sessionId={sessionId} />` inserted in the
`.app-chrome` header immediately before `<ThemeToggle />`, after the
`.chrome-spacer` flex gap. This places it at the right edge of the chrome,
left of the ThemeToggle and AuthorDot — consistent with the SessionList
integration that uses the same pattern in `.page-actions`.

**Props.** `variant` left at default (`'inline'`), matching the SessionList
integration. `sessionId` is the component's existing prop — passed through
unchanged.

**Test approach.** Two new tests in the existing `describe('SessionViewShell')` block:
1. "renders AttachHelpLink in the SessionViewShell chrome" — asserts
   `getByRole('button', { name: /setup help/i })` is present immediately
   on render (no async needed; the button is always in the DOM).
2. "forwards the sessionId to AttachHelpLink and the walkthrough dialog
   displays it" — renders with `sessionId='sess-42'`, clicks the help
   button, waits for the dialog (`role=dialog`), then asserts
   `/jamsesh:join sess-42` appears in the rendered walkthrough CC pane.

`beforeEach` additions: `localStorage.clear()` (prevents dismiss-flag
bleed between tests) and `Object.defineProperty(navigator, 'clipboard', ...)`
stub (jsdom has no clipboard API; the walkthrough's copy handler calls it).

**Negative-case verification.** Temporarily removed `<AttachHelpLink ...>`
from the chrome and ran the suite. Both new tests failed (`× renders
AttachHelpLink`, `× forwards the sessionId`) while the 12 pre-existing
SessionViewShell tests continued to pass. Restored the element; all 520
tests green on two consecutive runs.
