---
id: gate-docs-ux-nonexistent-slash-commands
kind: story
stage: implementing
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
