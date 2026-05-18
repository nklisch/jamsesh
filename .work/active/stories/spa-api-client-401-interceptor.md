---
id: spa-api-client-401-interceptor
kind: story
stage: done
tags: [ui]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# SPA API client lacks a 401 interceptor

## Finding

Discovered during e2e implementation of
`epic-e2e-tests-failure-mode-spa-error-states`. The Svelte SPA's API
client at `frontend/src/lib/api/client.ts` doesn't intercept 401
responses from the portal — when a stale bearer token hits a protected
endpoint and the backend returns 401, the SPA's auth state isn't
cleared and the user isn't redirected to login.

The `App.svelte` auth guard checks `localStorage.jamsesh.token`
presence (non-null), not validity. A non-null but stale token passes
the guard, the request hits the backend, and the 401 is left to the
calling component to handle individually (which it generally doesn't).

## Why it matters

User-visible UX gap: when a user's session expires (token revoked,
TTL expired), they see an opaque error or a hung page instead of being
returned to login. The expected flow is: 401 → `auth.signOut()` →
redirect to `/login`.

## Suggested implementation

Add a response interceptor in `frontend/src/lib/api/client.ts` that:
1. Calls `auth.signOut()` (clears localStorage)
2. Triggers a router push to `/login`
3. Optionally surfaces a brief "session expired" toast

## Acceptance criteria

- [ ] API client routes all 401 responses through `auth.signOut()`
- [ ] User lands on `/login` after a 401
- [ ] The Playwright test
      `stale_bearer_token_on_API_call_triggers_401_sign_out_and_login_redirect`
      in `tests/e2e/playwright/error-states.spec.ts` is un-skipped and
      passes

## Notes

The test in `error-states.spec.ts` is documented with a `test.skip`
and a comment pointing at this story. Re-enable when this lands.

## Implementation notes

Added a second openapi-fetch `Middleware` to
`frontend/src/lib/api/client.ts`:

```ts
const unauthorizedMiddleware: Middleware = {
  onResponse({ response }) {
    if (response.status === 401) {
      auth.signOut();
    }
  },
};
```

`auth.signOut()` already clears `jamsesh.token` + `jamsesh.refresh`
from localStorage and calls `navigate('/login')`, so the interceptor
is a single line of policy. The skipped "session expired" toast
mentioned in the original suggested-implementation is left out — no
toast infrastructure exists in the codebase yet, and the redirect-to-
login pattern is unambiguous enough that an extra toast adds noise
without information.

### No carve-out for /api/auth/* endpoints

Considered but rejected. A 401 from `/api/auth/refresh` means the
refresh token is also dead → the user must sign in fresh → `signOut`
is the correct action. A 401 from `/api/auth/magic-link/exchange`
means the link is bad → the user is not yet signed in → `signOut`'s
clear+navigate is a no-op for the clear part and reaches the right
end state (on /login). For OAuth start/callback and revoke a 401 is
either impossible or a no-op; the interceptor remains safe.

### Files touched

- `frontend/src/lib/api/client.ts` — added `unauthorizedMiddleware`
- `frontend/src/lib/api/client.test.ts` — added a `client — 401
  interceptor` describe block with four cases (401 clears + redirects;
  200 doesn't clear; 500 doesn't clear; parallel 401s are idempotent)
- `tests/e2e/playwright/error-states.spec.ts` — un-skipped the
  "stale bearer token..." test and refreshed the section comment

### Verification

`cd frontend && npm test -- --run src/lib/api/client.test.ts` — all
7 tests pass (3 existing Bearer-middleware + 4 new 401-interceptor).
Playwright test requires the running portal so re-enablement will be
proved out in CI's next e2e run.
