---
id: portal-dep-failure-error-codes-oauth
kind: story
stage: implementing
tags: [portal]
parent: portal-dep-failure-error-codes
depends_on: [portal-dep-failure-error-codes-envelope-helper]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Wire OAuth-provider HTTP failures to `dep.oauth_provider_unavailable`

Wraps `Provider.Exchange` failures in `auth/oauth.go` (OauthCallback)
so a GitHub OAuth provider that returns non-2xx (or a transport
failure) surfaces as
`{error: "dep.oauth_provider_unavailable"}` at HTTP 503 with
`Retry-After: 10`.

This is distinct from existing business 400s (`oauth.invalid_state`,
`oauth.expired_state`, `oauth.provider_mismatch`,
`oauth.unknown_provider`) which remain 400 and 503 for
`oauth.provider_not_configured` at startup — those are *config* errors,
not runtime *dep* errors.

## Files

- **Edit** `internal/portal/auth/oauth.go` — `OauthCallback`:

  Current:

  ```go
  ghIdentity, err := provider.Exchange(ctx, code, stateRow.RedirectURI)
  if err != nil {
      return nil, fmt.Errorf("oauth callback: exchange: %w", err)
  }
  ```

  Target:

  ```go
  ghIdentity, err := provider.Exchange(ctx, code, stateRow.RedirectURI)
  if err != nil {
      return nil, deperr.WrapOAuthProvider(
          fmt.Errorf("oauth callback: exchange: %w", err))
  }
  ```

  The `provider.Exchange` interface (see `internal/portal/oauth/provider.go`)
  bundles all HTTP/transport failures into a single returned error,
  so wrapping at this single site catches:
  - Token-exchange non-2xx (500/503 from GitHub, this is the e2e
    test's case)
  - Token-exchange transport failures (DNS, connection refused,
    timeout)
  - `/user` profile lookup failures (same provider, same wrap layer)
  - `/user/emails` failures

- **Edit** `internal/portal/oauth/github.go` if any of the inner HTTP
  call helpers swallow context or return ambiguous errors. Audit pass
  expected to be a no-op — the existing impl returns annotated errors
  for each step.

- **Edit** `internal/portal/auth/oauth_test.go` — find the existing
  `TestOauthCallback_ExchangeFailure_*` test (or add one if missing).
  Drive it with a stub provider whose `Exchange` returns
  `errors.New("github returned 503")`; assert:
  - HTTP 503
  - `Content-Type: application/json; charset=utf-8`
  - Body decodes to `{error: "dep.oauth_provider_unavailable"}`
  - `Retry-After: 10` header

## Acceptance criteria

- [ ] `OauthCallback` wraps `provider.Exchange` errors with
      `deperr.WrapOAuthProvider`
- [ ] Existing business 400 paths (`oauth.invalid_state`,
      `oauth.expired_state`, `oauth.provider_mismatch`,
      `oauth.unknown_provider`) are unchanged
- [ ] `StartOAuth` 503 `oauth.provider_not_configured` is unchanged
      (config error, not dep)
- [ ] Unit test asserts on
      `{error: "dep.oauth_provider_unavailable", status: 503,
      Retry-After: "10"}`
- [ ] `go test ./internal/portal/auth/...` passes

## Test approach

The existing test helper `auth.NewOAuthHandler` accepts a provider
map. Build a `stubFailingProvider` whose `Exchange` returns a fixed
error, and route the request through the strict handler with the
translator wired (matching the e2e setup). Assert on the envelope.

## Risk

LOW. Single-site wrap. The existing oauth flow continues to work for
the happy path and for every existing business-error branch.

## Rollback

`git revert`. Same as the other story 2/4/5 patches — independent.
