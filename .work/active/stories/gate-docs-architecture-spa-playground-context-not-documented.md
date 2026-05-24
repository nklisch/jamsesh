---
id: gate-docs-architecture-spa-playground-context-not-documented
kind: story
stage: drafting
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# ARCHITECTURE.md does not describe the SPA-side anonymous bearer / PlaygroundContext storage that landed in this bundle

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/ARCHITECTURE.md` (no current mention of SPA-side playground identity state)
- Code: `frontend/src/lib/auth.svelte.ts:13-21,37,63-69`

## Current doc text
> ARCHITECTURE.md describes the binary's `${CLAUDE_PLUGIN_DATA}/sessions/<id>/token` per-session bearer storage (lines 160-177) but says nothing about the SPA-side counterpart.

## Reality
The SPA's `auth` rune store carries a `_playgroundContext = $state<PlaygroundContext | null>(null)` field (`{sessionId, bearer, nickname}`) that is in-memory only (no localStorage), orthogonal to the authenticated-user identity, and exposed via `auth.playgroundContext` / `auth.setPlaygroundContext(...)`. The `story-foundation-doc-drift-bearer-storage-architecture` story rolled forward the CLI side but did not cover SPA storage.

## Required edit
Add a short paragraph to `docs/ARCHITECTURE.md` (under "Portal frontend" or as a peer subsection within the existing local-state coverage) describing that the SPA's `auth` rune store holds in-memory-only anonymous playground context (`{sessionId, bearer, nickname}`), separate from the localStorage-backed durable OAuth tokens, and that a tab reload drops the playground identity (intentional — the bearer is destroyed when the session ends).
