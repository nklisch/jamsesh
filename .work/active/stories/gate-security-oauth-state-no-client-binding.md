---
id: gate-security-oauth-state-no-client-binding
kind: story
stage: drafting
tags: [security]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: security
created: 2026-05-20
updated: 2026-05-20
---

# OAuth state nonce held only by backend; client has no tab-binding

## Severity
Low

## Domain
Authentication & Authorization

## Location
`frontend/src/lib/screens/Login.svelte:60-65`, `frontend/src/lib/screens/OAuthCallback.svelte:33-43`

## Evidence
```ts
sessionStorage.setItem('oauth.provider', 'github');
if (returnTo) {
  sessionStorage.setItem('oauth.return_to', returnTo);
} else {
  sessionStorage.removeItem('oauth.return_to');
}
```

## Remediation direction
The client persists only `oauth.provider` and `oauth.return_to` in
sessionStorage; the CSRF-defeating `state` nonce is held entirely
server-side and the client doesn't keep its own copy to cross-check.

If the callback ever runs in a tab/session different from the one that
initiated the flow (login-CSRF where an attacker tricks a victim into
completing an attacker-initiated OAuth login), the client cannot detect
the mismatch — it relies fully on the backend's state-binding.

Defense-in-depth: at OAuth start, persist a fresh client-side
correlation id (random UUID) in sessionStorage; have the backend echo
the same id into the callback (or include it in the authorize-url
`state`); at callback, assert the values match before posting to
`/api/auth/oauth/callback`. Reject otherwise.
