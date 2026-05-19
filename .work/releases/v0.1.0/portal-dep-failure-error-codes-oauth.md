---
id: portal-dep-failure-error-codes-oauth
kind: story
stage: done
tags: [portal]
parent: portal-dep-failure-error-codes
depends_on: [portal-dep-failure-error-codes-envelope-helper]
release_binding: v0.1.0
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

## Implementation notes

### Files touched

- `internal/portal/auth/oauth.go` — added `jamsesh/internal/portal/deperr`
  import; wrapped the `provider.Exchange` error with
  `deperr.WrapOAuthProvider` per the design. No other call site in the
  file was a dep-class wrap candidate (`ConsumeState` failures route
  through `store.ErrNotFound` to a 400, `FindOrProvision` errors are
  DB-class and owned by the sibling `portal-dep-failure-error-codes-db`
  story, `tokensSvc.Issue` is internal token logic, not a runtime dep).
- `internal/portal/auth/oauth_test.go`:
  - Imported `deperr`, `httperr`, and `strings`.
  - Switched `newOAuthTestEnv` (and the unconfigured-provider sub-test)
    from `openapi.NewStrictHandler(...)` to
    `openapi.NewStrictHandlerWithOptions(...)` with
    `httperr.WriteFromError` wired as the response-error handler — the
    test pipeline now mirrors production (`cmd/portal/main.go`).
  - Replaced the legacy `TestOAuthCallback_ExchangeError_Returns500`
    with `TestOAuthCallback_ExchangeError_ReturnsDepEnvelope`: asserts
    HTTP 503, `Content-Type: application/json; charset=utf-8`,
    `Retry-After: 10`, and `error = dep.oauth_provider_unavailable`.
  - Added `TestOauthCallback_WrapsExchangeError_WithDepSentinel` as a
    unit-level check that the handler returns an error matching
    `deperr.ErrOAuthProvider` and that the original transport-error
    string survives in the rendered error message.

### Internal-HTTP audit of `internal/portal/oauth/`

Per the story's "audit pass expected to be a no-op", reviewed
`github.go`'s three call helpers (`exchangeCode`, `fetchUser`,
`fetchPrimaryEmail`). Every error path already returns an annotated
error wrapped in `&ErrExchange{Provider, Cause}` from
`GitHub.Exchange`. No code changes required inside the provider
package — the single wrap at the OauthCallback call site catches all
of them.

### Parked gap: provider error taxonomy

`Provider.Exchange` currently bundles transport/HTTP failures *and*
RFC 6749 business errors (e.g. token endpoint 200 OK with
`{"error":"bad_verification_code"}`) into one `*ErrExchange` return.
The wrap at OauthCallback treats every failure as dep-class — correct
for transport, slightly mis-classifying for `invalid_grant`. Parked as
`.work/backlog/portal-oauth-provider-error-taxonomy.md` (sketch:
introduce an `oauth.ErrBadGrant` sentinel in the provider package,
classify in OauthCallback before wrapping). No active test depends on
the distinction in v1.

### Verification

- `go test ./internal/portal/auth/... ./internal/portal/oauth/...`:
  PASS (45 tests, 0 failures across both packages).
- `go build ./...`: PASS.

## Review

**Verdict: Approve.**

Implementation matches design exactly. Single-site wrap in
`OauthCallback` catches all transport/HTTP failure paths through
`Provider.Exchange` (token exchange, `/user`, `/user/emails`, decode).
Business 400 paths (`oauth.invalid_state`, `oauth.expired_state`,
`oauth.provider_mismatch`, `oauth.unknown_provider`) untouched.
`oauth.provider_not_configured` startup 503 untouched. Test pipeline
now wires `httperr.WriteFromError` via `NewStrictHandlerWithOptions`,
mirroring production in `cmd/portal/main.go`. Integration test asserts
on status, content-type, `Retry-After`, and typed envelope code. Unit
test guards `errors.Is(err, deperr.ErrOAuthProvider)` and message
preservation. `go test ./internal/portal/auth/... ./internal/portal/oauth/...`
passes.

**Findings:** 0 blockers, 0 important, 0 nits.

**Parked taxonomy gap:** the agent correctly parked
`portal-oauth-provider-error-taxonomy` for the `bad_verification_code`
misclassification — a real but mild gap (SPA shows "retry in 10s"
instead of "start over"). Parking is the right call: scope of this
story was explicitly transport/HTTP wrapping; the business-error branch
needs its own design surface (provider-package sentinel, classify-then-
wrap pattern, PROTOCOL.md addition). Backlog item is well-formed —
clear idea, approach sketch, deferral rationale, promotion triggers.
