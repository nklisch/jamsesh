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
