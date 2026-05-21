---
id: gate-security-401-blanket-signout
kind: story
stage: review
tags: [security]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: security
created: 2026-05-20
updated: 2026-05-20
---

# 401 response triggers blanket signOut for any API call

## Severity
Low

## Domain
Authentication & Authorization / Error Handling

## Location
`frontend/src/lib/api/client.ts:21-27` (referenced from bundle; bundle wires `auth.signOut()` at `auth.svelte.ts:53` as the global 401 handler)

## Evidence
```ts
const unauthorizedMiddleware: Middleware = {
  onResponse({ response }) {
    if (response.status === 401) {
      auth.signOut();
    }
  },
};
```

## Remediation direction
While `client.ts` itself is not in this bundle's diff, the bundle wires
`auth.signOut()` (which clears tokens and navigates) as the global 401
handler. Any endpoint that returns 401 for a per-resource authorization
reason (e.g. a stale per-org token) will silently sign the user out and
discard their refresh token.

Tighten the trigger so it fires only on auth-domain 401s — e.g. require
the typed error envelope's `error` field to be in a known auth-failure
set (`auth.token_expired`, `auth.token_invalid`, etc.) before invoking
`signOut`. Out-of-scope 401s should surface to the calling screen.

## Implementation notes

### Strategy chosen: prefix match `error.startsWith('auth.')`
Used a prefix match rather than an enumerated allowlist. This means any
future `auth.*` code the backend adds (e.g. `auth.token_revoked`,
`auth.session_invalidated`) will route through `signOut()` automatically
without a frontend code change. The current known codes are
`auth.invalid_token` and `auth.token_expired`.

### Body cloning
`openapi-fetch` shares the `Response` object with the caller. Since a
`Response` body is a single-shot stream, the middleware reads the body on
`response.clone()` so downstream callers retain the unconsumed original.
The `MiddlewareOnResponse` type in `openapi-fetch/dist/index.d.ts` (line
154-156) passes `Response` directly — no library-level body-reading
support — so cloning is the correct and only approach.

### Opaque-body fail-open
If the body can't be parsed as JSON (plain-text error pages, empty body,
network-level intercepts), the `catch` block leaves `errorCode` undefined
and the middleware does NOT call `signOut()`. The 401 surfaces to the
caller with its body intact. This is the safest default — a false-negative
(missed auth failure) is less disruptive than a false-positive (spurious
global signout on a per-resource 401).

### Tests added (frontend/src/lib/api/client.test.ts)
All added to the existing `client — 401 interceptor` describe block:

1. **non-auth 401 (error prefix not "auth.") does NOT trigger signOut** — body `{ error: 'org.scope_invalid' }`, 401. Asserts `signOut` not called, token preserved.
2. **opaque 401 (non-JSON body) does NOT trigger signOut** — plain-text body, 401. Asserts `signOut` not called, token preserved.
3. **auth.* subcode other than invalid_token triggers signOut** — body `{ error: 'auth.token_expired' }`. Asserts tokens cleared, navigated to /login.
4. **auth.* error on non-401 response (e.g. 403) does NOT trigger signOut** — body `{ error: 'auth.invalid_token' }`, status 403. Asserts `signOut` not called.

Existing tests retained and still valid: the first 401 test already used
`{ error: 'auth.invalid_token' }` (not a generic 401), so no update was
needed for them to reflect the new contract.

### Negative-case verification
Temporarily reverted `unauthorizedMiddleware` to the original blanket
`if (response.status === 401) { auth.signOut(); }`. Result:
- "non-auth 401 → no signOut" FAILED (as expected — blanket version fires on any 401)
- "opaque 401 → no signOut" FAILED (as expected — blanket version fires on any 401)
- All other tests still passed

Restored the new implementation. All 476 tests pass on two consecutive runs.

### Implementation discovery
The existing first test (`clears tokens and navigates to /login on 401 response`)
already used `{ error: 'auth.invalid_token' }` in its mock body — it was
written in anticipation of the tighter contract, or incidentally already
correct. No update was needed. The backend currently only emits 401s for
auth failures (no per-resource authorization 401s observed in the spec
or server code), so this story is defense-in-depth for a scenario that
could arise as the authorization model evolves.
