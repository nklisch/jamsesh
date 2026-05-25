---
id: idea-attach-onboarding-localstorage-error-handling
kind: story
stage: drafting
tags: [ui, bug]
parent: feature-attach-onboarding-a11y-robustness
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-21
updated: 2026-05-25
---

`SessionAttachWalkthrough.svelte:82` and `:90` call
`localStorage.setItem(DISMISS_KEY, 'true')` with no error handling.
Safari in Private Browsing throws `QuotaExceededError` on writes; some
sandboxed environments throw `SecurityError`. An uncaught throw inside
`handleClose` / `handleOpenSession` would prevent the modal from
closing — the user clicks Close, the parent's `onclose` is never
called, the modal stays mounted.

Found in the v0.3.1 review of `feature-portal-session-attach-onboarding`.

Fix shape: wrap each `setItem` in try/catch; on failure, log to console
but proceed with the close. Add a test that mocks `setItem` to throw
and asserts `onclose` still fires.

Note: the mode-on-mount read at `:46` uses `typeof localStorage !==
'undefined'` as a guard, which only catches the SSR case — `getItem`
itself can also throw in restricted contexts. Same try/catch treatment
applies.
