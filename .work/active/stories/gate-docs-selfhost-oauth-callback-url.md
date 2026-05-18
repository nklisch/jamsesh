---
id: gate-docs-selfhost-oauth-callback-url
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

# SELF_HOST.md §4 documents the GitHub OAuth callback URL as `/auth/github/callback` but the portal exposes `POST /api/auth/oauth/callback`

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SELF_HOST.md:270-273`
- Code: `docs/openapi.yaml:1521` (`POST /api/auth/oauth/callback`); the
  OAuth flow uses this single shared callback for all providers (the
  provider is encoded in the state nonce), per `internal/portal/oauth/`

## Current doc text
> 2. Set **Authorization callback URL** to:
>    ```
>    https://<your-portal-host>/auth/github/callback
>    ```

## Reality
The actual OAuth callback endpoint is `POST /api/auth/oauth/callback`
(a provider-agnostic JSON-body endpoint consumed by the SPA after
GitHub redirects back to the SPA's `/auth/callback` route). There is no
`/auth/github/callback` route in the portal.

## Required edit
Replace the example callback URL with the actual SPA redirect URL
(`https://<your-portal-host>/auth/callback?provider=github` or whatever
the SPA's redirect route is) and explain that the SPA forwards to
`POST /api/auth/oauth/callback` to exchange the code. Confirm against
the SPA before editing.

## Implementation notes

The prior rewrite (`gate-docs-selfhost-oauth-future-release`) updated §4 to
reference `POST /api/auth/oauth/callback`, but step 2 of "Registering the
GitHub OAuth app" still described a non-existent SPA-hop flow: it told
operators to enter the SPA origin and implied the SPA would POST the code
to the portal. Inspecting `frontend/src/lib/router.svelte.ts` confirmed
there is no `/auth/callback` SPA route; `Login.svelte` comments confirm
the OAuth exchange is a full-page server-side redirect chain — GitHub
redirects the browser directly to `POST /api/auth/oauth/callback` with no
SPA intermediary.

Fix applied: step 2 now shows the exact URL to paste into GitHub
(`https://<your-portal-host>/api/auth/oauth/callback`) and notes the
server-side exchange explicitly.
