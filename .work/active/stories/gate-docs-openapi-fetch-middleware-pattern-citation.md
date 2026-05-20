---
id: gate-docs-openapi-fetch-middleware-pattern-citation
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: docs
created: 2026-05-20
updated: 2026-05-20
---

# `openapi-fetch-middleware-client` pattern skill cites stale `auth.svelte.ts:53` example

## Drift category
pattern-skill-staleness

## Location
- Doc: `.claude/skills/patterns/openapi-fetch-middleware-client.md:48-55`
- Code: `frontend/src/lib/auth.svelte.ts:53` (now `signOut()` declaration, not a client call); the actual `client.GET('/api/me')` now lives at `frontend/src/lib/auth.svelte.ts:76` and intentionally destructures `{ data }` only.

## Current doc text
> ### Example 3: response destructure pattern is uniform
>
> **Files**: `frontend/src/lib/screens/FinalizeView.svelte:107`,
> `frontend/src/lib/components/CommentsTab.svelte:25`,
> `frontend/src/lib/components/NewSessionDrawer.svelte:41`,
> `frontend/src/lib/auth.svelte.ts:53` — every caller destructures
> `{ data, error }` from the awaited `client.{GET,POST,PATCH}(...)`.
>
> 13+ call sites across screens and components.

## Reality
The bundle extended `auth.svelte.ts` — the `client.GET('/api/me')` call
moved to line 76 and now intentionally destructures only `{ data }` (the
catch block + `if (data && _token === tokenAtStart)` guard cover the
error/race paths, so `error` is unused). Line 53 is now the `signOut()`
method declaration with no `client.` call at all. The "every caller
destructures `{ data, error }`" claim is also no longer literally true
(auth.svelte.ts is now a counter-example), and the new `Home.svelte`
`POST /api/orgs` call (`frontend/src/lib/screens/Home.svelte:41`) is a
fresh `{ data, error }` site that would be a stronger example.

## Required edit
Replace the `auth.svelte.ts:53` citation with
`frontend/src/lib/screens/Home.svelte:41` (a clean
`const { data, error } = await client.POST('/api/orgs', ...)` site) and
soften the claim so it accommodates the legitimate `{ data }`-only
auth.svelte.ts call:

> **Files**: `frontend/src/lib/screens/FinalizeView.svelte:107`,
> `frontend/src/lib/components/CommentsTab.svelte:25`,
> `frontend/src/lib/components/NewSessionDrawer.svelte:41`,
> `frontend/src/lib/screens/Home.svelte:41` — callers destructure
> `{ data, error }` from the awaited `client.{GET,POST,PATCH}(...)` (or
> `{ data }` alone when the error path is handled by middleware + an
> outer try/catch, as in `frontend/src/lib/auth.svelte.ts:76`).
>
> 13+ call sites across screens and components.
