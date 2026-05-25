---
id: idea-attach-onboarding-keyboard-accessibility
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

The walkthrough modal has several click-only interactive surfaces that
keyboard users cannot trigger:

- **`.term-line` elements** (`SessionAttachWalkthrough.svelte:152-179`) —
  the two shell-install command rows. `<!-- svelte-ignore
  a11y_click_events_have_key_events -->` suppresses the warning. A
  keyboard-only user can't copy the install commands.
- **`.cc-input` elements** (`:213-223`, `:294-303`) — the Claude Code
  join command row in both mode branches. Same suppression, same gap.
- **`.reopen-link` in compact mode** (`:315-319`) — the "First-time
  setup?" expansion link. Click-only.

Found in the v0.3.1 review of `feature-portal-session-attach-onboarding`.

Fix shape: convert each click-only interactive surface to a real
`<button>` (preferred — gets native keyboard handling for free, role,
focus ring) or add `onkeydown` handlers that fire on Enter/Space. For
the terminal lines specifically, a button wrapper around the
prompt+command keeps the visual but adds Enter-to-copy. Drop the
a11y-ignore comments once fixed.

Test addition: simulate Tab navigation through the modal and Enter on
each copyable line; assert clipboard receives the right command.
