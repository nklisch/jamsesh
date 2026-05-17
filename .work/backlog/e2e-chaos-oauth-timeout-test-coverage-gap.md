---
id: e2e-chaos-oauth-timeout-test-coverage-gap
kind: story
stage: drafting
tags: [e2e-test, testing, oauth]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E chaos test `oauth_provider_timeout` does not actually exercise the timeout

## Finding

`tests/e2e/chaos/network_and_provider_test.go > testOAuthProviderTimeout`
was un-skipped when `portal-oauth-client-timeout` landed the 15s HTTP client
timeout. The test now runs green, but the assertions pass for the wrong
reason.

## Why it matters

WireMock's delay (`tests/e2e/chaos/testdata/github_delay_10s.json`) is 10s.
The portal's `githubOAuthHTTPTimeout` is 15s. With 10s < 15s, the sequence
is:

1. Portal POSTs to WireMock `/login/oauth/access_token` at t=0
2. WireMock responds at t=10s with whatever the mapping defines (likely a
   non-token body or 200 with empty body)
3. Portal receives the response at t=10s and either errors out parsing it
   (e.g. "github returned empty access_token") or returns a non-2xx for
   another reason
4. Portal returns non-2xx to the test at ~t=10s

The portal's 15s timeout NEVER fires. The test asserts `elapsed <= 18s` and
`status != 200`, both of which pass with the WireMock-responds-at-10s shape
— but they would also pass if the portal had no timeout at all (or a 100s
timeout), as long as WireMock continues to return non-token bodies at 10s.

The doc comment introduced in commit `0031fe3` says:
> "The portal's 15s HTTP client timeout fires first; the callback returns
>  a non-2xx error within the configured timeout window."

That statement is factually wrong with the current WireMock delay value.

## Suggested fix

Pick one (or both):

1. **Increase WireMock delay to > portal timeout.** Change
   `github_delay_10s.json` to e.g. 30s (or 20s) so the portal's 15s
   timeout actually fires first. Rename the file to
   `github_delay_30s.json` and update the mapping path constant.
   This makes the test actually exercise the timeout path.

2. **Reduce the portal-side OAuth timeout for chaos-test runs** via
   `JAMSESH_OAUTH_GITHUB_TIMEOUT` env or `GitHubOptions` plumbing (if
   exposed). Then WireMock's 10s delay can stay; the test-only timeout
   would be e.g. 5s, so the timeout fires at 5s.

Approach 1 is simpler and matches the existing chaos-test pattern.

Additionally:

- Tighten the assertion to verify timeout-shaped behavior: assert
  `elapsed >= portalTimeout - margin` AND `elapsed <= portalTimeout + margin`
  (a band, not just an upper bound). This catches the case where WireMock
  returns much earlier than the timeout.
- Correct the doc comment introduced in `0031fe3` to accurately describe
  what the test exercises (or, if approach 1 is taken, the comment becomes
  accurate as-is).

## Acceptance criteria

- [ ] WireMock delay > portal OAuth timeout, so the timeout actually fires
- [ ] Test asserts elapsed in a band around the portal timeout (not just
      an upper bound)
- [ ] Test doc comment accurately describes the timing relationship

## References

- `tests/e2e/chaos/network_and_provider_test.go > testOAuthProviderTimeout`
- `tests/e2e/chaos/testdata/github_delay_10s.json`
- `internal/portal/oauth/github.go > githubOAuthHTTPTimeout`
- Implementation that introduced the gap: commit `0031fe3` (review of
  `portal-oauth-client-timeout`)
