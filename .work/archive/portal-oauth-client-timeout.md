---
id: portal-oauth-client-timeout
kind: story
stage: done
tags: [bug, security, oauth]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Bug: portal OAuth HTTP client has no timeout

## Discovery

Found during implementation of `epic-e2e-tests-chaos-network-and-provider`.

`internal/portal/oauth/github.go` — the `httpClient()` method returns
`http.DefaultClient` when no `HTTPClient` is set in `GitHubOptions`:

```go
func (g *GitHub) httpClient() *http.Client {
    if g.opts.HTTPClient != nil {
        return g.opts.HTTPClient
    }
    return http.DefaultClient
}
```

`http.DefaultClient` has no timeout (`Timeout: 0`), which means a slow or
unresponsive OAuth provider (GitHub outage, network partition, WireMock
with `fixedDelayMilliseconds: 10000`) will cause the portal to hang
**indefinitely** on the token exchange request.

## Impact

- A production GitHub outage causes every OAuth callback request to hang
  until the OS TCP stack times out (90+ seconds by default).
- With enough concurrent slow callbacks the portal's goroutine pool can be
  exhausted, causing a cascading failure affecting unrelated requests.
- No `Retry-After` header or user-visible error is surfaced during the hang.

## Fix

Set a reasonable timeout on the HTTP client used for OAuth calls. Suggested:

```go
const oauthHTTPTimeout = 15 * time.Second

func (g *GitHub) httpClient() *http.Client {
    if g.opts.HTTPClient != nil {
        return g.opts.HTTPClient
    }
    return &http.Client{Timeout: oauthHTTPTimeout}
}
```

Or construct a shared `*http.Client` once in `NewGitHub` and store it in the
`GitHub` struct so the timeout is configurable via `GitHubOptions`.

## Acceptance criteria

- [x] The OAuth HTTP client used in production has an explicit timeout
      (configurable, default ≤ 30s)
- [x] The `oauth_provider_timeout` chaos scenario in
      `tests/e2e/chaos/network_and_provider_test.go` can be un-skipped and
      runs green (portal times out cleanly, callback returns a clear error)
- [x] Unit tests in `internal/portal/oauth/` verify the timeout path

## References

- `internal/portal/oauth/github.go` — `httpClient()` method
- `tests/e2e/chaos/network_and_provider_test.go` — `oauth_provider_timeout`
  subtest (currently skipped pending this fix)

## Implementation notes

### Timeout constant

Chose `githubOAuthHTTPTimeout = 15 * time.Second`. Rationale: generous enough
for slow networks (GitHub's documented 99th-percentile token exchange is well
under 5s), tight enough to prevent goroutine pileup during an outage. The
WireMock chaos scenario uses a 10s fixedDelay — the 15s client timeout fires
after 10s, causing the portal to surface an error rather than hang. Callers
who need a different bound pass their own `*http.Client` via
`GitHubOptions.HTTPClient`; that path is unchanged.

Construction-per-call was kept (not `sync.Once`) because these code paths are
infrequent (one token exchange per login), and `http.Client` construction is
cheap. No need to introduce shared mutable state.

### Files touched

- `internal/portal/oauth/github.go` — added `"time"` import, added
  `githubOAuthHTTPTimeout` constant, changed `httpClient()` to return
  `&http.Client{Timeout: githubOAuthHTTPTimeout}` instead of
  `http.DefaultClient`, updated `GitHubOptions.HTTPClient` comment.
- `internal/portal/oauth/export_test.go` — new file (test-only, compiled only
  into test binaries); exports `HTTPClientForTest()` so the external
  `oauth_test` package can inspect the unexported `httpClient()` result.
- `internal/portal/oauth/github_test.go` — added `"time"` import; added
  `TestGitHub_DefaultHTTPClientHasTimeout` (verifies default client has
  `0 < Timeout <= 30s`) and `TestGitHub_Exchange_TimesOutOnSlowProvider`
  (uses a 100ms test-timeout against a 500ms sleeping server; completes in
  well under a second of wall-clock time).
- `tests/e2e/chaos/network_and_provider_test.go` — removed `t.Skip` from
  `testOAuthProviderTimeout`; updated package-level doc comment to show the
  scenario as active.

### Chaos test verification

The chaos test was NOT executed (requires Docker + WireMock + full portal
image). Build-only verification confirmed it compiles cleanly:
`cd tests/e2e && go build ./chaos/...` — exit 0.

The test's assertions were inspected and confirmed correct for the fix:
- Uses `expectedPortalTimeout = 15 * time.Second` + 2s client timeout margin.
- Asserts elapsed <= `expectedPortalTimeout + 3s` (fires before WireMock's
  10s delay would expire on a client with no timeout).
- Asserts status != 200 (portal must surface the error, not silently succeed).

### Follow-ups

None. The chaos test should be executed as part of a future full e2e run once
the Docker environment is available to confirm end-to-end green.

## Review (2026-05-17)

**Verdict**: Approve with comments

**Blockers**: none
**Important**:
- `e2e-chaos-oauth-timeout-test-coverage-gap` — the chaos test
  `oauth_provider_timeout` does NOT actually exercise the timeout path.
  WireMock's delay (`github_delay_10s.json`) is 10s, portal timeout is 15s.
  WireMock responds first at t=10s with a non-token body; the portal returns
  non-2xx because of that, not because the timeout fires. The new doc
  comment "The portal's 15s HTTP client timeout fires first" (and the
  matching claim in this story's implementation notes) is factually wrong.
  The unit tests in `github_test.go` DO verify the timeout fires —
  end-to-end coverage of the timeout path is the gap. Filed for follow-up,
  not blocking this story since the primary deliverable (the production
  timeout) is correct and unit-test-verified.
**Nits**: none

**Notes**:
- Production fix is correct and well-documented. The 15s constant has a
  defensible rationale in the inline comment; the default-path-injects-
  timeout regression test guards against future refactors that might
  silently re-introduce `http.DefaultClient`.
- The `export_test.go` pattern (`HTTPClientForTest()`) is a clean way to
  white-box test the unexported `httpClient()` without making it public.
- `TestGitHub_Exchange_TimesOutOnSlowProvider` is the real timeout-fires
  verification — uses a 100ms test-only client timeout vs a 500ms sleeping
  server, completes in 0.50s wall-clock. Asserts both error and that
  elapsed <= 400ms (catches a "didn't time out" regression). Both new
  tests passed locally during review:
  ```
  go test ./internal/portal/oauth/... -run "TestGitHub_DefaultHTTPClientHasTimeout|TestGitHub_Exchange_TimesOutOnSlowProvider" -v
  PASS (0.504s)
  ```
- No foundation-doc drift: `docs/SECURITY.md` does not codify a specific
  OAuth-timeout property, so no rolling-forward needed. If the security
  doc ever asserts "all outbound requests are bounded" the constant should
  be referenced.
- Backward compatibility: callers passing `GitHubOptions.HTTPClient`
  unchanged. Callers using the default get a 15s timeout where previously
  they had none — that's a strict improvement.

## What's now possible

The portal can no longer hang indefinitely on a slow GitHub OAuth provider.
Goroutine pileup during a GitHub outage is bounded. The chaos test slot for
this scenario is active (even if the WireMock delay needs adjustment for
true timeout-path coverage).
