---
id: gate-docs-ux-nonexistent-slash-commands
kind: story
stage: review
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: docs
created: 2026-05-18
updated: 2026-05-18
---

# UX.md references `/jamsesh:create` and `/jamsesh:sync` slash commands that do not exist

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/UX.md:33` and `docs/UX.md:119`
- Code: `skills/` only ships `jamsesh, join, status, fork, mode, finalize`.
  `cmd/jamsesh/main.go:25-37` registers `auth, mcp-headers, hook, join,
  status, fork, mode, finalize, finalize-run` — no `create`, no `sync`.

## Current doc text
> 1. From a checkout of the source repo, they run `/jamsesh:create`.
> …
> 6. In CC, run `/jamsesh:sync` or the agent's next prompt — the local
>    checkout is updated to reflect the fork.

## Reality
Session creation happens via the portal UI (POST
`/api/orgs/{orgID}/sessions`); there is no CLI/skill surface for
creation. Forking auto-updates the local checkout — there is no
`/jamsesh:sync` step.

## Required edit
Rewrite the "creating a session" flow around the portal UI as the entry
point (or document an explicit CLI flow only if it exists). Remove the
`/jamsesh:sync` step or replace it with the actual mechanism (e.g. the
next agent turn picks up the new ref binding automatically).

## Implementation notes

Two edits made to `docs/UX.md`:

**"Flow: creating a session" (was lines 33–45):** Replaced the
`/jamsesh:create` step with a portal-UI entry point. The new flow has the
user click "New session" in the portal, fill in the session parameters
(name, goal, scope, mode, invitees), click Create, and then run
`/jamsesh:join <session-id-or-url>` from their local checkout — the same
join flow every collaborator uses. This matches `POST
/api/orgs/{orgID}/sessions` as the sole creation surface and
`sessioncmd.JoinCommand()` as the local-binding mechanism.

**"Flow: forking from a peer" step 6 (was line 119):** Replaced
`/jamsesh:sync` (nonexistent) with accurate prose: the fork is created
server-side; the `UserPromptSubmit` hook fetches `session-remote` at the
start of every agent turn (`cmd/jamsesh/hooks/userpromptsubmit.go:169`),
so the new ref is available locally on the next turn without any manual
step.
