---
id: idea-attach-onboarding-dialog-role-on-card
kind: story
stage: drafting
tags: [ui, a11y]
parent: feature-attach-onboarding-a11y-robustness
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-21
updated: 2026-05-25
---

`SessionAttachWalkthrough.svelte:112-121` puts `role="dialog"`,
`aria-modal="true"`, and `aria-label="Attach Claude Code to this jam"`
on the `.modal-backdrop` wrapper element rather than on the
`.modal-card` `<article>` itself. Screen readers expect the dialog
landmark to wrap the actual dialog content, not the dimmed overlay
region around it. Effects:

- VoiceOver / NVDA may announce the dialog as encompassing the full
  viewport rather than just the card.
- Keyboard focus trap helpers that look for `[role="dialog"]` see the
  backdrop, not the content surface.

Compare with `frontend/src/lib/components/Modal.svelte:58-63` which
correctly puts the role on the inner `<div class="modal">`.

Found in the v0.3.1 review of `feature-portal-session-attach-onboarding`.

Fix shape: move `role="dialog"`, `aria-modal="true"`, `aria-label`,
and `tabindex` to the inner `<article class="modal-card">`. Keep the
backdrop a plain `<div>` with the click-to-close handler. Make sure
the a11y-ignore comments stay scoped to where they're actually
needed.
