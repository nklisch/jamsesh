---
id: gate-docs-selfhost-oauth-callback-url
kind: story
stage: drafting
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
