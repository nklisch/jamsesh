---
id: bug-frontend-oauth-callback-handler-missing
kind: story
stage: done
tags: [bug, ui, auth]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-19
---

# OAuth flow's second hop has no SPA handler — `/auth/oauth/callback` 404s

## Symptom

Even with `bug-frontend-oauth-start-route-mismatch` fixed, GitHub
sign-in still fails to complete. After the user authorizes on
GitHub, the browser is redirected to
`https://<portal>/auth/oauth/callback?code=...&state=...` (the
`redirect_uri` the backend builds at
`internal/portal/auth/oauth.go:74`). The SPA router has no matching
route, falls through to `not-found`, and renders the `<NotFound />`
screen. The `code`+`state` are never POSTed to
`/api/auth/oauth/callback`; tokens are never issued; the user is
never signed in.

This is the SECOND of two route-shape bugs in the OAuth flow. The
first (`/api/auth/oauth/github/start` 404) was fixed by
`bug-frontend-oauth-start-route-mismatch`. This second one was uncovered
during that story's review.

## Root cause

The v0.1.0 OAuth epic (`epic-portal-foundation-auth-flows-oauth-provider-github`)
scoped only the backend handlers (`StartOAuth`, `OauthCallback` in
`internal/portal/auth/oauth.go`) plus their OpenAPI schemas and tests.
The frontend SPA-hop handler — the equivalent of
`MagicLinkExchange.svelte` for the OAuth callback — was never built.

The expected flow (documented in `docs/SELF_HOST.md` §4 after
`bug-docs-oauth-callback-url-and-flow-prose-mismatch` landed) is:

1. SPA POSTs `/api/auth/oauth/start` → backend mints state nonce →
   returns `authorize_url`.  ✓ done by sibling story.
2. SPA navigates browser to `authorize_url` (GitHub). ✓ done.
3. User authorizes on GitHub.
4. GitHub redirects browser to
   `<portal>/auth/oauth/callback?code=...&state=...` — **the SPA must
   own this route, parse the query, and POST to the backend.**  ✗ missing.
5. SPA POSTs `/api/auth/oauth/callback` with `{provider, code, state}`
   → backend validates nonce, exchanges code for identity, issues
   `TokenPair`.
6. SPA stores tokens via `auth.setTokens()` and navigates to landing
   or `?return_to=`.

Steps 4–6 have no implementation in the SPA. The existing
`MagicLinkExchange.svelte` (frontend/src/lib/screens/MagicLinkExchange.svelte)
is the exact analog for the magic-link flow and is the model to mirror.

Additionally, the in-file LIMITATION comment at
`frontend/src/lib/screens/Login.svelte:33-39` documents the WRONG flow
architecture — it asserts "GitHub OAuth is a full-page server-side
redirect chain" and "the browser never executes JavaScript between
GitHub → /api/auth/oauth/callback → the client." That contradicts
both the actual code (callback is `POST`, requires JS to invoke) and
the documented flow in `docs/SELF_HOST.md` §4. The comment's wrong
premise produced its wrong conclusion about `return_to` propagation —
in fact, once the SPA owns step 4, `return_to` CAN be propagated
trivially (via sessionStorage set in `signInWithGitHub` and read back
in the callback screen).

## Fix approach

Mirror the magic-link pattern.

1. **New screen** `frontend/src/lib/screens/OAuthCallback.svelte`,
   modeled on `MagicLinkExchange.svelte`:
   - `onMount`: read `code` + `state` from `window.location.search`
     (these are query params, not the hash — OAuth doesn't use
     fragments).
   - POST `client.POST('/api/auth/oauth/callback', { body: { provider, code, state } })`.
   - Provider value: pull from `sessionStorage` (set in
     `signInWithGitHub` before redirecting to GitHub). The backend
     also validates provider against the state-stored value, so the
     SPA can hardcode `'github'` as a fallback safely.
   - On 200: `auth.setTokens(data.access_token, data.refresh_token)`,
     then `navigate(returnTo ?? '/login')` reading `return_to` from
     sessionStorage too (set alongside provider in
     `signInWithGitHub`).
   - On error: show error UI mirroring MagicLinkExchange.svelte's
     error state — display the typed `error` code envelope.
2. **Router** `frontend/src/lib/router.svelte.ts`: add
   `{ pattern: /^\/auth\/oauth\/callback$/, name: 'oauth-callback', params: [] }`.
3. **App.svelte**:
   - Import and dispatch `OAuthCallback` on `current.name === 'oauth-callback'`.
   - Exclude `'oauth-callback'` from the auth-gate redirect (same
     treatment as `'magic-link'`).
4. **Login.svelte**:
   - In `signInWithGitHub`, BEFORE the POST: write
     `sessionStorage.setItem('oauth.provider', 'github')` and, if a
     `return_to` query param was preserved, write
     `sessionStorage.setItem('oauth.return_to', returnTo)`. These survive
     the GitHub round-trip.
   - Replace the LIMITATION comment at lines 33-39 with an accurate
     short description of the SPA-hop, or remove it entirely.
5. **Test** (`frontend/src/lib/screens/OAuthCallback.test.ts`):
   - Happy path: query has `code`+`state` → POST sent with correct
     body → tokens stored → navigated to `return_to` (or `/login`
     fallback).
   - Error path: backend 400 → error UI shown.
   - Missing-code path: no `code` in query → error UI shown without
     POST.

## Implementation notes

### Files created
- `frontend/src/lib/screens/OAuthCallback.svelte` — new screen mirroring MagicLinkExchange.svelte. Reads `code`+`state` from query params, reads `oauth.provider` and `oauth.return_to` from sessionStorage, clears both before POSTing, exchanges with `/api/auth/oauth/callback`, stores tokens and navigates on success.
- `frontend/src/lib/screens/OAuthCallback.test.ts` — 14 tests covering all specified scenarios.

### Files modified
- `frontend/src/lib/router.svelte.ts` — added `oauth-callback` route adjacent to `magic-link`.
- `frontend/src/App.svelte` — imported OAuthCallback, excluded `oauth-callback` from auth gate, added template branch.
- `frontend/src/lib/screens/Login.svelte` — added sessionStorage writes for `oauth.provider` and `oauth.return_to` in `signInWithGitHub`; replaced stale LIMITATION comment with accurate description.

### Test scenarios covered (14 total)
1. Renders exchanging state on mount before POST resolves
2. Calls POST with correct provider/code/state
3. Stores tokens and navigates to `oauth.return_to` on success
4. Clears sessionStorage entries after reading
5. Falls back to `/login` when no `oauth.return_to` in sessionStorage
6. Uses `github` as provider fallback when `oauth.provider` absent
7. Shows `missing_params` error without POST when `code` absent
8. Shows `missing_params` error without POST when `state` absent
9. Shows backend error code when POST returns error envelope
10. Shows `exchange_failed` when POST returns error without code
11. Shows `exchange_failed` when POST throws (network failure)
12. Rejects protocol-relative `return_to` (`//evil.com`) and falls back to `/login`
13. Clears code+state from URL via `history.replaceState` after reading
14. Renders back-to-sign-in button in error state

### Non-obvious decisions
- sessionStorage keys are cleared eagerly (before the async POST) rather than after, so a failed exchange still clears stale keys and avoids replaying them on a subsequent visit.
- The component catches all `exchange` throws rather than just `TypeError`s, since any unexpected rejection should degrade to the same `exchange_failed` error UI.

## References

- Analog implementation:
  `frontend/src/lib/screens/MagicLinkExchange.svelte`
- Backend endpoint contract: `docs/openapi.yaml:1565`
  (`POST /api/auth/oauth/callback`)
- Backend handler: `internal/portal/auth/oauth.go:91-186`
- Backend `redirect_uri` it builds:
  `internal/portal/auth/oauth.go:74` —
  `redirectURI := h.portalURL + "/auth/oauth/callback"`
- Router file to extend: `frontend/src/lib/router.svelte.ts`
- App dispatcher: `frontend/src/App.svelte`
- Comment to rewrite: `frontend/src/lib/screens/Login.svelte:33-39`
- Documented flow: `docs/SELF_HOST.md` §4 (post
  `bug-docs-oauth-callback-url-and-flow-prose-mismatch`)
- Discovered during review of:
  `bug-frontend-oauth-start-route-mismatch`

## Review (2026-05-19)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `frontend/src/App.svelte:22-23` — the auth-gate exclusion comment
  mentions magic-link but not the new oauth-callback exclusion. The
  code is correct (both are excluded at line 25); only the comment
  drifted. Trivial follow-up the next time anyone touches that block.
- `OAuthCallback.svelte:60` — the empty catch block has no inline
  comment explaining the catch-all reasoning (it's in the story body
  but not in code). Consistent with `MagicLinkExchange.svelte`'s
  pattern, which also has no comment, so leaving as-is.

**Notes**: Clean mirror of the magic-link pattern. The fan of 14 tests
covers all the paths that matter: happy path (token storage +
return_to navigation), the two missing-param branches, backend error
envelope passthrough, generic fallback, network throw, and an
explicit open-redirect protection test for `//evil.com`. Mock setup
exactly mirrors `MagicLinkExchange.test.ts` so the convention holds.
Non-obvious decisions in the story body are both defensible —
clearing sessionStorage eagerly prevents stale-key replay on
subsequent visits to the callback URL, and catching all throws (not
just `TypeError`) means any unexpected rejection degrades to the same
user-visible error UI rather than leaking. Foundation-doc alignment
clean: the new flow matches `docs/SELF_HOST.md` §4 (which was already
rewritten by `bug-docs-oauth-callback-url-and-flow-prose-mismatch`),
and the stale LIMITATION comment in Login.svelte was rewritten to
match.

**What's now possible**: GitHub OAuth sign-in works end-to-end. A
self-hoster on the next portal release will complete the round-trip:
click "Continue with GitHub" → authorize on GitHub → land back at
`/auth/oauth/callback` → SPA exchanges the code → tokens stored →
into the sessions landing. Both 404s that were blocking the flow
(`bug-frontend-oauth-start-route-mismatch` and this story) are now
closed.
