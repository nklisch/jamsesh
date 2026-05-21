---
id: story-portal-session-attach-onboarding-sessionviewshell-affordance
kind: story
stage: implementing
tags: [ui]
parent: feature-portal-session-attach-onboarding
depends_on: [story-portal-session-attach-onboarding-help-link]
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
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
