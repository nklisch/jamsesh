---
id: gate-docs-pattern-openapi-fetch-middleware-stale-anchors
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# Pattern skill `openapi-fetch-middleware-client.md` cites several stale `*.svelte:NNN` example anchors

## Drift category
pattern-skill-staleness

## Location
- Doc: `.claude/skills/patterns/openapi-fetch-middleware-client.md:29,40,50-53,56`
- Code: `frontend/src/lib/api/client.ts`, `frontend/src/lib/components/CommentsTab.svelte`, `frontend/src/lib/components/NewSessionDrawer.svelte`, `frontend/src/lib/screens/FinalizeView.svelte`, `frontend/src/lib/auth.svelte.ts`

## Current doc text
> Cites `client.ts:39`, `TreeDag.svelte:61`, `FinalizeView.svelte:107`, `CommentsTab.svelte:25`, `NewSessionDrawer.svelte:41`, `Home.svelte:41`, `auth.svelte.ts:76`.

## Reality
Bundle's frontend god-component decomposition (`feature-refactor-frontend-god-components`) refactored `FinalizeView`, `CommentsTab`, `NewSessionDrawer` extensively; per-line anchors all drifted. `FinalizeView.svelte:107` is now `orgId,` (a parameter, not an openapi-fetch call); `CommentsTab.svelte:25` is a comment; `NewSessionDrawer.svelte:41` is `onclose();`; `auth.svelte.ts:76` is `},`. `client.ts:39` is now `try {`. Only `Home.svelte:41` and `TreeDag.svelte:61` happen to still resolve.

## Required edit
Re-grep for `client.GET|client.POST|client.PATCH|client.DELETE` across the current `frontend/src/lib/` tree and replace the stale `:NNN` anchors with present line numbers (or, more durable, swap line-number anchors for symbol-based pointers like "`Home.svelte` createOrg call" so the pattern survives future refactors).
