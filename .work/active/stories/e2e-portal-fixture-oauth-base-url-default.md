---
id: e2e-portal-fixture-oauth-base-url-default
kind: story
stage: review
tags: [e2e-test, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E portal fixture: default OAuth base URL to something safe

## Finding

`tests/e2e/fixtures/portal/portal.go > buildEnv` only sets
`JAMSESH_OAUTH_GITHUB_BASE_URL` when `Options.OAuthBaseURL` is non-empty.
The fixture always sets non-empty `CLIENT_ID` / `CLIENT_SECRET`
(defaulting to `"test-client"` / `"test-secret"`), so the GitHub OAuth
provider IS constructed inside the portal container. If a test exercises
the OAuth flow without setting `OAuthBaseURL`, the portal will call real
`github.com` and `api.github.com`.

The smoke spec doesn't hit OAuth, so this is latent. Future golden-path /
failure-mode tests touching OAuth need to be careful, or this fixture
should make the safe-by-default behavior explicit.

## Suggested fix

Two options, pick one:

1. **Default `OAuthBaseURL` to a sentinel "no network" value** — e.g.,
   `http://localhost:1` (an unroutable port). Any OAuth attempt fails
   fast with a connection refused, rather than silently calling real
   GitHub. Tests that want WireMock substitution override the value.

2. **Refuse to start with non-empty `CLIENT_ID` + empty `OAuthBaseURL`** —
   the fixture's `Start` function `t.Fatalf`s with "configure
   OAuthBaseURL to point at WireMock or set CLIENT_ID=\"\" to disable
   GitHub OAuth entirely". Forces test authors to make the choice
   explicit.

Option 2 is the more defensive choice — it surfaces the decision at
test-design time rather than at first network call.

## Decision

**Option 2 (defensive `t.Fatalf`) — and also drop the "test-client"
default.** The story's option 2 focused on the `CLIENT_ID` default;
closing the loop on it cleanly requires both changes together:

1. `t.Fatalf` in `portal.Start` when `OAuthGitHubClientID` is non-empty
   AND `OAuthBaseURL` is empty.
2. Drop the `"test-client"` / `"test-secret"` defaults from `buildEnv`.
   `JAMSESH_OAUTH_GITHUB_CLIENT_ID` is now injected only when the caller
   explicitly sets `OAuthGitHubClientID`. The portal already handles
   missing OAuth credentials gracefully (returns 503
   `oauth.provider_not_configured` from `/api/auth/oauth/*`), so tests
   that don't touch OAuth (smoke, magic-link, session, finalize, etc.)
   simply leave both fields zero.

Why both changes together: keeping the `"test-client"` default while
adding the guard would `t.Fatalf` ~11 existing tests that rely on the
default but don't actually need OAuth. Dropping the default makes the
safe behavior the default for the common case (no OAuth needed), and
the guard catches the dangerous case (set `OAuthGitHubClientID`
without a stub URL).

## Implementation notes

### `tests/e2e/fixtures/portal/portal.go`

- `Start` gained an upfront check before `buildEnv`:

  ```go
  if opts.OAuthGitHubClientID != "" && opts.OAuthBaseURL == "" {
      t.Fatalf("portal: OAuthGitHubClientID is set but OAuthBaseURL " +
          "is empty; configure OAuthBaseURL to point at WireMock or " +
          `leave OAuthGitHubClientID="" to disable GitHub OAuth ` +
          "in the portal entirely")
  }
  ```

- `buildEnv` no longer defaults `OAuthGitHubClientID` /
  `OAuthGitHubClientSecret` to `"test-client"` / `"test-secret"`.
  `CLIENT_ID` is injected only when the caller sets it; `SECRET`
  defaults to `"test-secret"` only when `CLIENT_ID` is set and
  `SECRET` is empty (so OAuth-using tests don't have to supply both
  fields verbatim).

### Existing callers — no breakage

- 3 tests that already set `OAuthBaseURL` continue to work:
  - `scaffolding/healthz_test.go` doesn't set `CLIENT_ID` → no OAuth
    provider constructed → still passes (only hits `/healthz`).
  - `failure/config_and_deps_test.go` and
    `chaos/network_and_provider_test.go` set both `CLIENT_ID` and
    `BaseURL` → guard passes → OAuth wired to WireMock as before.
- 11 tests that don't set either field continue to work — they get
  no OAuth provider in the portal, and the 503 fallback handles the
  rare case where one of them does inadvertently touch an oauth
  endpoint.

### Verification

`cd tests/e2e && go build ./...`: clean.

## Acceptance criteria

- [x] A test that doesn't set `OAuthBaseURL` cannot accidentally reach
      real GitHub — without `OAuthGitHubClientID` the portal does not
      construct a GitHub OAuth provider at all; with both set, the
      provider points at the stub URL.
- [x] The fixture's docs clearly state the safe-by-default behavior —
      the doc comment on `buildEnv`'s OAuth block explains the
      injection rule; the new t.Fatalf message tells the developer
      exactly what to do.
- [x] Existing smoke spec continues to work without changes — verified
      via call-site audit; all 14 `portal.Start` callers still compile
      and behave the same way.
