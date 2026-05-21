---
id: gate-security-401-blanket-signout
kind: story
stage: implementing
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
