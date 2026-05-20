---
id: gate-tests-oauthcallback-loadme-rejection
kind: story
stage: drafting
tags: [testing]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: tests
created: 2026-05-20
updated: 2026-05-20
---

# OAuthCallback navigation when `loadCurrentUser` rejects — spec-silent failure path

## Priority
Medium

## Spec reference
Item: `spa-logged-in-landing-authed-redirect-fixes`
Acceptance criterion: Notes section — "If `auth.loadCurrentUser()` rejects or hangs, the OAuthCallback path must STILL navigate — the `await` is inside a `try` block that already exists. Verify the catch branch still navigates / surfaces error UI as before; do not silently swallow load failures past the existing error handling shape."

## Gap type
adversarial-spec-silent / error case

## Suggested test
```ts
it('navigates to / even when loadCurrentUser rejects', async () => {
  mockLoadCurrentUser.mockRejectedValueOnce(new Error('boom'));
  mockPOST.mockResolvedValue({
    data: { access_token: 'a', refresh_token: 'r', /* ... */ },
    error: null,
  });
  sessionStorage.removeItem('oauth.return_to');
  render(OAuthCallback);
  await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/'));
  // setTokens still fired — user is signed in even though /api/me failed.
  expect(mockSetTokens).toHaveBeenCalled();
});
```

## Test location (suggested)
`frontend/src/lib/screens/OAuthCallback.test.ts`

## Context
The implementation note "Verified that `loadCurrentUser` swallows all
exceptions internally" relies on cross-module knowledge — if
`loadCurrentUser`'s internal try/catch is ever refactored away, the
OAuthCallback would jump to the outer catch and the user would see
"exchange_failed" despite tokens being set. A test pinning
"loadCurrentUser rejection still navigates and keeps tokens" is the only
way to fail-fast at the OAuthCallback boundary if that contract changes.
