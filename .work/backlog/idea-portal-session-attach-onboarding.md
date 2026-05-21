---
id: idea-portal-session-attach-onboarding
created: 2026-05-20
tags: [ui, plugin]
---

The portal UI gives users no guidance on how to actually attach a local
Claude Code instance to a freshly-created session: the NewSessionDrawer
just closes after a successful POST, and the SessionViewShell drops the
user into the awareness surface as if they were already participating.
The designed flow in `docs/UX.md` (Phase "Flow: creating a session" /
"Flow: joining a session") calls for the portal to return a join URL +
expects users to run `/jamsesh:join <session-id-or-url>` from a checkout,
but the SPA never tells anyone that — they have to already know.

**Desired shape** (per user direction, 2026-05-20):

- **First-time setup walkthrough** on first session create/join. Steps:
  1. `claude plugin marketplace add nklisch/jamsesh` (the CC marketplace add)
  2. `claude plugins install jamsesh` (the plugin install)
  3. `/jamsesh:join <session-id>` from a checkout of the source repo
  - Should explain plainly that the user's local CC needs the plugin
    before the join command works, plus a reminder to be `cd`'d into a
    checkout of the source repo when running it.
- **"Don't show again" checkbox** in the walkthrough. Persist per
  account (or fall back to localStorage if backend-side preference
  storage isn't yet a thing). Once dismissed, drop to the condensed view.
- **Condensed view for experienced users**: just the `/jamsesh:join
  <session-id>` line + copy button + a small "First-time setup?" link
  that re-opens the full walkthrough on demand. No marketplace/plugin
  steps cluttering the everyday flow.
- **Always-reachable reference**: even after "don't show again", an
  affordance somewhere persistent — session-list header or
  SessionViewShell chrome — to re-open the walkthrough when a user
  forgets the command or sets up a new machine.

**Likely scope**:

- Frontend: new `SessionAttachWalkthrough` component + integration in
  `NewSessionDrawer` success state and `SessionViewShell` "not yet
  attached" state. Probably a top-level account-preferences slot for
  the "don't show again" flag.
- Backend: extend the `Session` schema in `docs/openapi.yaml` to
  include a `join_command` (or join URL) field so the UI doesn't have
  to hardcode the slash-command pattern. May also need an
  account-preferences endpoint for the persisted dismissal flag if we
  don't want to rely on localStorage alone.
- Docs: README "Install the Claude Code plugin" section (just landed
  in v0.3.0) overlaps — should cross-reference or get folded into the
  in-app walkthrough as the canonical source.

**Open questions for scope time**:

- Does "first time" mean first session created/joined PER ACCOUNT, or
  per BROWSER? Account-scoped is more useful (covers multi-device) but
  needs a backend preference field. localStorage is cheap but resets
  on every new device/incognito.
- Does the walkthrough also belong in invite-accept (a collaborator
  receiving a join link)? Probably yes — same install steps apply.
- Does the "attach" affordance need to know whether the current user
  HAS attached yet (i.e., whether their `jam/<session>/<user>/main`
  ref exists)? If so the backend needs to expose that, or the UI
  needs to listen for first-push via WS and update its state.
- Should the walkthrough also offer a "I want to spectate from the
  portal only" path, given the portal IS a first-class surface per
  UX.md? Or is that out of scope for the attach-onboarding flow
  (spectating doesn't need a join).

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
- Iteration notes (for the eventual feature design):
  1. First pass (Option 2) made first-time and returning views look like
     two unrelated screens — confused the user. Refined into Option 5
     where both states are the same modal-card shape (just bigger / smaller).
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
