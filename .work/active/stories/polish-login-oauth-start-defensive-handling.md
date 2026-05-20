---
id: polish-login-oauth-start-defensive-handling
kind: story
stage: review
tags: [bug, ui, auth]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-19
---

# Login GitHub button — drop double-submit, handle network throws, fix stale comment

## Brief

Three follow-up items surfaced during review of
`bug-frontend-oauth-start-route-mismatch` that I did not bundle into
the original fix to keep its diff scoped. Rolling them up here as a
small polish commit.

## Items

1. **Inaccurate comment** at `frontend/src/lib/screens/Login.svelte:55-58`.
   The new comment introduced by the original fix says the start
   endpoint cannot 302 because "the nonce must be allocated by an
   **authenticated** SPA call before redirection." The endpoint is
   explicitly `security: []` (`docs/openapi.yaml:1538`) — it is not
   authenticated. Drop the inaccurate justification; the two-step
   nature is enough WHY on its own.

2. **No double-submit guard** on the GitHub button. Rapid clicks fire
   two `POST /api/auth/oauth/start` calls, mint two state nonces, and
   orphan one in the `oauth_state` table until its 5-minute TTL
   expires. Mirror the `isSubmitting` pattern used in
   `frontend/src/lib/components/NewSessionDrawer.svelte:38` —
   short-circuit if already in flight, disable the button while
   pending.

3. **No try/catch around `client.POST`**. openapi-fetch returns
   `{error}` on non-2xx HTTP (already covered by the 503 test), but
   on a real network failure (offline, CORS, DNS) `fetch()` throws and
   the promise rejection is unhandled. The button visibly hangs with
   no error UI. Wrap the call so a throw routes through the same
   `'oauth-error'` UI as a non-2xx response, and add a test that mocks
   `fetch` to reject.

## Acceptance

- [ ] Comment at `Login.svelte:55-58` no longer claims the OAuth-start
      call is authenticated.
- [ ] `signInWithGitHub` short-circuits if already in flight; button
      is `disabled` while pending.
- [ ] `signInWithGitHub` wraps `client.POST` so a `fetch` throw routes
      to the existing `'oauth-error'` UI instead of leaking an
      unhandled rejection.
- [ ] New test: `fetch` rejects → error UI shown, no unhandled rejection.
- [ ] Existing 9 Login tests still pass; full frontend suite still
      green; svelte-check clean.

## Implementation notes

- `frontend/src/lib/screens/Login.svelte`:
  - Removed the "authenticated SPA call" sentence from the comment
    above `signInWithGitHub`; the remaining two-sentence WHY is
    accurate and sufficient.
  - Added `oauthPending = $state(false)`.
  - `signInWithGitHub` now short-circuits when `oauthPending` is true,
    wraps the `client.POST` in a `try { ... } catch {}`, and funnels
    both the `error`/`!data` branch AND a `fetch` throw through a
    single error-set site at the bottom (cleaner than duplicating
    `mode = 'oauth-error'; errorMsg = ...` in two places).
  - GitHub button is `disabled={oauthPending}`; added `:disabled`
    style (cursor not-allowed, 0.6 opacity) and scoped the `:hover`
    rule to `:not(:disabled)` so the disabled button doesn't still
    show a hover background.
- `frontend/src/lib/screens/Login.test.ts`:
  - New test `'OAuth button routes a fetch throw to the error UI'` —
    mocks `globalThis.fetch` to reject with `TypeError('Failed to fetch')`,
    asserts the error UI renders.
  - New test `'OAuth button only fires one start request on rapid double-click'` —
    blocks the fetch response with a manually-resolved promise so the
    button stays in-flight across three clicks, asserts `fetch` is
    called exactly once and the button is `disabled`.

Verification:
- `npx vitest run src/lib/screens/Login.test.ts` — 11/11 pass.
- `npx vitest run` (full suite) — 391/391 pass (was 389; +2 new).
- `npx svelte-check` — 0 errors (2 pre-existing unrelated warnings).
