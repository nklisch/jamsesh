---
id: story-portal-session-attach-onboarding-help-link
kind: story
stage: implementing
tags: [ui]
parent: feature-portal-session-attach-onboarding
depends_on: [story-portal-session-attach-onboarding-walkthrough-component]
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
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
