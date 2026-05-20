---
id: bug-frontend-oauth-start-route-mismatch
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

# Login page's "Continue with GitHub" hits a non-existent route

## Symptom

Clicking "Continue with GitHub" on the production login page navigates
to `https://jamsesh.dev/api/auth/oauth/github/start` and renders the
typed error envelope:

```
{"error":"route.not_found","message":"no route matches"}
```

OAuth sign-in is therefore completely broken for self-hosters on
v0.1.2 and earlier.

## Root cause

`frontend/src/lib/screens/Login.svelte:55` does a hand-rolled top-level
navigation:

```ts
window.location.assign('/api/auth/oauth/github/start');
```

That URL pattern doesn't exist anywhere in the backend. The OpenAPI
contract (`docs/openapi.yaml:1534`) defines a single OAuth-start
operation:

```yaml
/api/auth/oauth/start:
  post:
    operationId: startOAuth
    requestBody: { provider: "github" }
    responses:
      200: { authorize_url: "https://github.com/login/oauth/authorize?..." }
```

OAuth start is intentionally a two-step flow:

1. Client POSTs to `/api/auth/oauth/start` with `{provider}` →
   backend mints a state nonce, persists it with TTL, and returns the
   provider's `authorize_url` (already carrying the nonce).
2. Client navigates the browser (`window.location.assign(authorize_url)`)
   to GitHub. The backend does NOT 302-redirect from start, because
   the SPA must execute the call to allocate the nonce that ends up
   in the URL.

`Login.svelte` skipped step 1 entirely and synthesized a URL pattern
that never existed. The bundled `frontend/src/lib/api/client.ts`
(openapi-fetch + bearer/401 middleware) was the right call site;
the OAuth button was the only place in the SPA bypassing it.

The existing test (`Login.test.ts:44`) codified the wrong URL as
expected, so the regression had no guard:

```ts
expect(assignSpy).toHaveBeenCalledWith('/api/auth/oauth/github/start');
```

That's a stale assertion (test debt, not a separate product bug) and
is repaired in-session.

## Fix approach

1. Replace the hand-rolled `window.location.assign(...)` with a typed
   `client.POST('/api/auth/oauth/start', { body: { provider: 'github' } })`.
2. On success, `window.location.assign(data.authorize_url)` to actually
   send the browser to GitHub.
3. On error (network, 400 unknown provider, 503 not configured), surface
   a user-visible message via the existing error template; add an
   `'oauth-error'` mode so the template branch is honest about which
   flow failed.
4. Repair `Login.test.ts` — the previous assertion was wrong; new
   assertions verify the typed POST body and that the returned
   `authorize_url` is the value passed to `location.assign`.

The OAuth contract itself is unchanged. This is purely a frontend wiring
fix that aligns the SPA with the existing OpenAPI contract.

## Regression test

`frontend/src/lib/screens/Login.test.ts` —
`'OAuth button posts to /api/auth/oauth/start and assigns the returned authorize_url'`:

- Mocks `globalThis.fetch` to return
  `{authorize_url: 'https://github.com/login/oauth/authorize?state=abc'}`.
- Clicks the GitHub button.
- Asserts the request URL ends with `/api/auth/oauth/start`, method is
  POST, body is `{"provider":"github"}`.
- Asserts `window.location.assign` is called with the returned
  authorize URL.

A second test covers the dep-failure branch (503 from the start endpoint
→ generic "Could not start GitHub sign-in" error UI), exercising the
new `'oauth-error'` mode and the existing `{:else}` template branch.

## Implementation notes

Files changed:

- `frontend/src/lib/screens/Login.svelte` —
  - Import shared typed client (`$lib/api/client`).
  - Add `'oauth-error'` to the `Mode` union; the existing `{:else}`
    error-template branch displays it identically to
    `'magic-link-error'`.
  - Rewrite `signInWithGitHub` to do
    `client.POST('/api/auth/oauth/start', { body: { provider: 'github' } })`,
    surface a user-visible error on failure, and
    `window.location.assign(data.authorize_url)` on success.
- `frontend/src/lib/screens/Login.test.ts` — replaced the stale
  assertion against the broken URL with a wire-shape test against the
  real route, plus a new failure-mode test.

Verification:

- `npx vitest run src/lib/screens/Login.test.ts` — 9/9 pass.
- `npx vitest run` (full frontend suite) — 389/389 pass.
- `npx svelte-check` — 0 errors (2 pre-existing warnings unrelated).

No backend changes — the OpenAPI contract was already correct; the SPA
was the only thing out of step.

Adjacent issues noticed but NOT bundled (these stay in this story body
as context for future cleanup; not parked as separate items because
they're trivial and tracked via the existing in-file TODO comment at
`Login.svelte:9-10`):

- The magic-link form in `Login.svelte` still uses a raw `fetch()` to
  `/api/auth/magic-link/request`. The in-file comment already flags
  this is deferred until that endpoint lands in `docs/openapi.yaml`
  under `epic-portal-foundation-auth-flows`. Out of scope here.
- The `'magic-link-error'` and `'oauth-error'` modes render identical
  UI. If a third flow is added, collapse to a single `'error'` mode.
  Not worth the rename today.

## Review (2026-05-19)

**Verdict**: Approve with comments

**Blockers**: none

**Important**:
- **Second hop of OAuth flow is also broken — `/auth/oauth/callback`
  has no SPA route** (`frontend/src/lib/router.svelte.ts`,
  `frontend/src/App.svelte`). My fix repairs hop 1 (start), but
  after GitHub redirects the browser to
  `<portal>/auth/oauth/callback?code=...&state=...`, the SPA router
  has no matching route and renders `NotFound`. Tokens are never
  exchanged; OAuth sign-in is still broken end-to-end. The story's
  brief sentence "OAuth sign-in is therefore completely broken" was
  literally true pre-fix AND remains true post-fix for a *different*
  reason. The v0.1.0 `epic-portal-foundation-auth-flows-oauth-provider-github`
  scoped only backend handlers; the SPA-hop screen (analog of
  `MagicLinkExchange.svelte`) was never built.
  → Item: `bug-frontend-oauth-callback-handler-missing`
- **Stale architecture comment in `Login.svelte:33-39`** —
  the LIMITATION block claims "the browser never executes JavaScript
  between GitHub → /api/auth/oauth/callback → the client," which
  contradicts the actual `POST`-shaped callback and the SPA-hop flow
  documented in `docs/SELF_HOST.md` §4 (per
  `bug-docs-oauth-callback-url-and-flow-prose-mismatch`). I did not
  touch this comment in the fix to keep the diff scoped, but its
  premise is wrong and its `return_to`-can't-be-propagated conclusion
  is also wrong (once the SPA owns the callback route it can read
  `return_to` from sessionStorage written before the GitHub redirect).
  Folded into the same parked item — naturally addressed when the
  callback screen lands.
  → Bundled in: `bug-frontend-oauth-callback-handler-missing`

**Nits**:
- `'magic-link-error'` + `'oauth-error'` modes render identical UI; a
  future cleanup could collapse to a single `'error'` mode. Already
  noted in implementation notes above.

**Notes**: The fix does exactly what its brief specified — closes the
start-route 404 the user reported. The end-to-end OAuth-broken
symptom has a second cause that was not visible from the user's
report (because the user never got past the first 404 to discover the
second one). Approve and advance; the second-hop gap is parked as a
ready-to-implement story with the `MagicLinkExchange.svelte` analog
called out as the model.
