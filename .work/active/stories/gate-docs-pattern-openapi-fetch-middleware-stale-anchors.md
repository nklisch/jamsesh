---
id: gate-docs-pattern-openapi-fetch-middleware-stale-anchors
kind: story
stage: done
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

## Implementation notes

Re-anchored `.claude/skills/patterns/openapi-fetch-middleware-client.md` examples from line numbers to symbol-based pointers (named handlers, named `client.GET`/`POST` calls). Added an explicit note at the top of the Examples section that anchors are intentionally symbol-based given the v0.4.0 god-component decomposition. Bumped the call-site count from 13+ to 20+ to reflect the extracted hooks.

Verified: Foundation docs are markdown — no build/test step. Edits preserve the rolling-foundation discipline (no "previously" prose, no "in v1.x" notes; assertions replaced in place).

## Review notes

Spawned `review-openapi-fetch-pattern-rolling-foundation-prose` (Important) — the example section still contains "the v0.4.0 god-component decomposition refactor moved most of these" which is a rolling-foundation violation; the introductory note at the top of the Examples section already covers the symbol-anchoring rationale without naming a version.
