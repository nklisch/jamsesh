---
id: feature-portal-session-attach-onboarding
kind: feature
stage: drafting
tags: [ui]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# Portal session-attach onboarding

## Brief

The portal UI today gives users no guidance on how to actually attach a
local Claude Code instance to a freshly-created (or freshly-joined)
session. `NewSessionDrawer` just closes after the POST succeeds, and
`SessionViewShell` drops the user into the awareness surface as if they
were already participating. The designed flow in `docs/UX.md` calls for
the portal to surface a join command after session creation and expects
users to run `/jamsesh:join <session-id>` from a checkout of the source
repo, but the SPA never tells anyone that — they have to already know.

This feature closes that gap by introducing a tiered-disclosure attach
walkthrough that appears at every point a user encounters a fresh session
they haven't joined yet. First-time users get a ceremonial three-step
walkthrough; experienced users get a compact one-liner with the join
command; an always-reachable "Setup help" affordance in the portal chrome
re-opens the full walkthrough at any time.

Visual direction is locked: the modal embeds a Claude-Code-styled pane
(real `claudecode-color.svg` icon, CC's slate-navy chrome, `#D97757`
accent, `❯` prompt indicator) that distinguishes the join slash command
from the two preceding shell-command install steps. See `## Mockups`
below.

## Strategic decisions

Resolved at scope time:

- **Persistence of "don't show again"**: per-browser via `localStorage`
  (no backend). Multi-device users may see the walkthrough once per
  browser; acceptable as a "remember-me" niceness. Avoids growing scope
  into a backend account-preferences slice.
- **Invite-accept inclusion**: in-scope. The walkthrough also opens after
  a successful invite-accept — same install steps apply for collaborators
  arriving via a join link.
- **Affordance behavior**: dumb (always available, same UI for everyone).
  The chrome affordance does not detect whether the current user has
  already attached. Lower complexity; avoids needing a backend attach-state
  endpoint or WS-listening to first-push.

## Anchor surfaces

Four touchpoints in the SPA, listed in expected child-story order:

1. **`SessionAttachWalkthrough` shared component** — the modal itself,
   per the locked mock. State machine for first-time / compact / re-opened.
   localStorage flag `jamsesh.attach-walkthrough-dismissed`. Click-to-copy
   for both shell commands and the slash command. Real CC icon embedded.
2. **`NewSessionDrawer` integration** — on POST success, open the
   walkthrough (passing the new session id) instead of just closing.
3. **`InviteAccept` integration** — on accept success, open the
   walkthrough (passing the session id from the invite).
4. **Always-reachable chrome affordance** — a "Setup help" link in
   `SessionList` and `SessionViewShell` chrome that opens the walkthrough
   on demand. Always available; not gated on attach-state.

## Open scope-time questions (deferred to feature-design)

These are design-internal calls — feature-design Phase 4 / Phase 4.5
resolves them when it drafts the child stories:

- **Compact-view trigger**: dismissal-flag-only (always show full on first
  load until dismissed) or also auto-compact when the user re-opens the
  modal mid-session (within the same browser tab)? Subtle UX call.
- **CC pane "running" feedback**: should the click-to-copy interaction
  also pretend to "run" the command (e.g., a fake "session joined" line
  appears), or is just-copy enough? Mock shows just-copy; revisit if user
  testing surfaces confusion.
- **SessionViewShell empty-state vs always-on**: should the in-session
  affordance render differently when there's clearly no commits yet
  (suggesting the user hasn't attached) vs when commits are flowing
  (clearly attached)? The "dumb" strategic decision says no — but
  presentation could still vary cosmetically.

## Mockups

- Compare: `.mockups/screens/portal-session-attach-onboarding/index.html`
- **Selected: option-6** — Terminal-first ceremonial, both states sharing
  the same modal shell. Signed off 2026-05-20.
- Rationale: Two shell commands in a "your terminal" pane on top, then a
  Claude-Code-styled pane below for the slash command. The CC pane embeds
  the real `claudecode-color.svg` brand icon, uses CC's `#D97757` clay
  accent, a white `❯` prompt indicator, and matches CC's slate-navy
  chrome. The compact (returning) state is the same shell at a smaller
  size — single CC pane, single command. Surface distinction
  (shell vs Claude Code) is named explicitly in the lede prose and
  reinforced by the visual chrome of each pane.
- Iteration notes (for feature-design's reference):
  1. First pass (Option 2) made first-time and returning views look like
     two unrelated screens — confused the user. Refined into Option 5
     where both states are the same modal-card shape (just bigger /
     smaller). Option 6 carried that journey shape but adopted the
     terminal-styled CC pane.
  2. Three layout bugs caught during iteration: eyebrow stretching to
     full width (column-flex `align-items: stretch`); third step-card
     overflowing past modal edge (`min-width: auto` on flex/grid
     children); rust-tinted status bar reading wrong (matched real CC's
     slate bg).
  3. Surface labeling matters: original mocks rendered all three
     commands with a shell `$ ` prompt. The join command runs *inside*
     Claude Code, not in a shell. Final design splits the panes
     visually (shell pane top, CC pane bottom) and uses `❯` (CC's
     prompt char) for the slash command.
  4. CC pane brand fidelity: hand-drawn mascot wasn't close enough; the
     real `claudecode-color.svg` is now embedded. Use CC's own colors
     (`#D97757` for accent, white for the prompt chevron) inside the
     pane, but keep the rest of the portal modal in Quiet Slate.
  5. Long-command overflow: `.cc-input .cc-cmd` uses
     `text-overflow: ellipsis` rather than `overflow-x: auto` so the
     compact view doesn't grow a horizontal scrollbar; the copy still
     fires from `data-cmd` (full string, never truncated).

## Notes for feature-design

- No foundation-doc roll-forward — UX.md already describes the flows
  this feature surfaces; the implementation is a faithful rendering, not
  a directional shift.
- No backend / `docs/openapi.yaml` changes — the join command is composed
  client-side from the session id the SPA already has.
- Tests live alongside each child story per the project's
  `spa-test-module-mock-barrel` pattern; no separate testing story.
- The README "Install the Claude Code plugin" section (shipped in v0.3.0)
  carries the same install commands — should stay aligned with whatever
  the in-app walkthrough says.
