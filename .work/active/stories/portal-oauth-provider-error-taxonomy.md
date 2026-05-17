---
id: portal-oauth-provider-error-taxonomy
kind: story
stage: review
tags: [portal, auth, error-taxonomy]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Distinguish OAuth business errors from transport/dep failures

## Idea

`internal/portal/oauth.Provider.Exchange` (and the GitHub
implementation) currently bundles every failure path into a single
`*oauth.ErrExchange` wrapping. Surfaced during the
`portal-dep-failure-error-codes-oauth` story: the OauthCallback wrap
site now treats *all* `Exchange` errors as dep-class
(`dep.oauth_provider_unavailable`, 503, Retry-After 10s).

That is correct for the common cases (network failure, GitHub 5xx, /user
or /user/emails lookup failure, decode failure, empty access_token). It
is *not* strictly correct for one path:

- `github.go:171-173` — when GitHub's token endpoint returns 200 OK
  with a JSON body like `{"error":"bad_verification_code",
  "error_description":"..."}`. RFC 6749 calls this an
  `invalid_grant` / `bad_verification_code` — a business error
  meaning the user's authorization code is expired, malformed, or
  already used. Today it surfaces as `dep.oauth_provider_unavailable`
  503, which suggests "GitHub is down, retry in 10s" to the SPA — the
  retry will never succeed because the issue is the user's code.

The honest contract would be 400 `oauth.invalid_grant` (or similar
business code) for this path, keeping the dep envelope for genuine
transport / 5xx / decode failures.

## Approach sketch

1. Add a sentinel in `internal/portal/oauth`:
   ```go
   var ErrBadGrant = errors.New("oauth: provider rejected the authorization code")
   ```
   (or a typed `*BadGrantError` that carries the upstream
   error/error_description).

2. Update each provider's `Exchange` to return `ErrBadGrant`-wrapped
   errors when the upstream response is structurally a business
   rejection (token endpoint 200 + `error` field for GitHub; the
   spec's `invalid_grant` / `bad_verification_code` codes).

3. In `auth/oauth.go > OauthCallback`, classify before wrapping:
   ```go
   if errors.Is(err, portaloauth.ErrBadGrant) {
       return oauthBadRequest("oauth.invalid_grant",
           "authorization code was rejected by the provider"), nil
   }
   return nil, deperr.WrapOAuthProvider(...)
   ```

4. Register `oauth.invalid_grant` in
   `docs/PROTOCOL.md > HTTP error contract` as a 400 business code,
   matching the existing `oauth.invalid_state` / `oauth.expired_state`
   pattern.

5. Unit test: stub `Provider.Exchange` returning `ErrBadGrant`-wrapped
   error, assert 400 with `error = oauth.invalid_grant`.

## Why deferred

- The current `portal-dep-failure-error-codes-oauth` story scope is
  narrow ("wrap transport/HTTP failures") — adding the business-error
  branch is a new code path with its own design surface.
- The misclassification's user-facing symptom is mild: the SPA shows
  "OAuth provider unavailable, retry in 10s" instead of "your sign-in
  link expired, start over" — annoying, not broken. Users will
  re-initiate sign-in on their own.
- No active test depends on this distinction yet.

## When to promote

- A failure-mode e2e test wants to assert behaviour for
  `bad_verification_code` (likely in
  `tests/e2e/failure/oauth_*_test.go` if/when added).
- An operator metrics dashboard wants to break out "OAuth provider
  down" from "OAuth user error" — the current taxonomy hides the
  difference in logs (the wrapped error string carries it, but the
  typed code does not).
- Multi-provider support is added (Google, GitLab, etc.) — each
  provider has its own business-vs-transport boundary and an
  uncoordinated approach across providers will compound the
  classification debt.

## Implementation notes

**Sentinel shape.** Picked the simpler `var ErrBadGrant = errors.New(...)`
plus `fmt.Errorf("%w: ...", ErrBadGrant, ...)` style — matches the existing
`deperr.Err*` convention (sentinel + thin wrap helper). No typed
`*BadGrantError` carrying structured fields; the upstream `error` /
`error_description` are preserved in the wrap text for operator logs,
which is enough for v1. If a future caller needs to programmatically
inspect the upstream code (e.g. distinguish `invalid_grant` from
`redirect_uri_mismatch`) we can promote to a typed error then.

**Wrap-chain interaction.** `ErrBadGrant` lives at the leaf of the
`*ErrExchange` chain — `Exchange` wraps the provider error in
`*ErrExchange{Cause: <ErrBadGrant-wrapped>}`. Since `*ErrExchange.Unwrap`
returns `Cause`, `errors.Is(err, oauth.ErrBadGrant)` traverses both
layers and returns true. The existing transport-failure cases (no
`ErrBadGrant`) continue to fall through to the dep-class wrap.

**Files touched.**
- `internal/portal/oauth/provider.go` — added `ErrBadGrant` sentinel
  next to `ErrExchange`.
- `internal/portal/oauth/github.go` — replaced the bare
  `fmt.Errorf("github error %s: %s", ...)` at the 200-OK-with-error
  branch with `fmt.Errorf("%w: ...", ErrBadGrant, ...)`.
- `internal/portal/auth/oauth.go` — added `errors.Is(err,
  portaloauth.ErrBadGrant)` classification before
  `deperr.WrapOAuthProvider`; returns
  `oauthBadRequest("oauth.invalid_grant", ...)` (400) on match.
- `internal/portal/oauth/github_test.go` — added three tests:
  `bad_verification_code` → `ErrBadGrant`, `invalid_grant` →
  `ErrBadGrant`, and transport failure does NOT match `ErrBadGrant`
  (taxonomy boundary guard).
- `internal/portal/auth/oauth_test.go` — added HTTP-surface test
  (400 + `oauth.invalid_grant` envelope, no `Retry-After` header)
  and unit-level test asserting the response type is
  `OauthCallback400JSONResponse` rather than a returned error that
  would route through the dep translator.
- `docs/PROTOCOL.md` — registered `oauth.invalid_grant` in the
  Common error codes section with a one-liner distinguishing it
  from `dep.oauth_provider_unavailable`.

**Tests.** All existing oauth + auth tests pass alongside the four
new ones. The existing `TestOAuthCallback_ExchangeError_ReturnsDepEnvelope`
still asserts 503 `dep.oauth_provider_unavailable` for plain transport
errors, confirming the new classification path doesn't catch the
fallthrough.
