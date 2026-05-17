---
id: portal-oauth-client-timeout
kind: story
stage: implementing
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

- [ ] The OAuth HTTP client used in production has an explicit timeout
      (configurable, default ≤ 30s)
- [ ] The `oauth_provider_timeout` chaos scenario in
      `tests/e2e/chaos/network_and_provider_test.go` can be un-skipped and
      runs green (portal times out cleanly, callback returns a clear error)
- [ ] Unit tests in `internal/portal/oauth/` verify the timeout path

## References

- `internal/portal/oauth/github.go` — `httpClient()` method
- `tests/e2e/chaos/network_and_provider_test.go` — `oauth_provider_timeout`
  subtest (currently skipped pending this fix)
