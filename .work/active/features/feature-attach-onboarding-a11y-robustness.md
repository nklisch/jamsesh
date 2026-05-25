---
id: feature-attach-onboarding-a11y-robustness
kind: feature
stage: drafting
tags: [ui, a11y, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# SessionAttachWalkthrough modal — a11y + robustness pass

## Brief

Close the four review nits surfaced during the v0.3.1 review of
`feature-portal-session-attach-onboarding`. All four touch the same file
(`frontend/src/lib/screens/SessionAttachWalkthrough.svelte`) and follow the
same shape: a missing error path, a misplaced ARIA role, or click-only
interaction. Bounded — single file, no design system shift, no foundation-doc
impact.

## Member stories

- `idea-attach-onboarding-clipboard-error-handling` —
  wrap `navigator.clipboard.writeText` in try/catch with graceful UI
- `idea-attach-onboarding-dialog-role-on-card` —
  move `role="dialog"` from backdrop to inner `<article class="modal-card">`
- `idea-attach-onboarding-keyboard-accessibility` —
  convert click-only `.term-line` / `.cc-input` / `.reopen-link` to real
  buttons; remove a11y-ignore suppressions
- `idea-attach-onboarding-localstorage-error-handling` —
  wrap `localStorage.setItem`/`getItem` in try/catch so QuotaExceededError
  / SecurityError don't keep the modal mounted

## Approach (high level)

Feature-design will refine, but these are parallel — no internal
sequencing. Tests should mock `clipboard.writeText`, `localStorage`, and
Tab/Enter keyboard navigation.
