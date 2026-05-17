---
id: e2e-portal-fixture-oauth-base-url-default
kind: story
stage: drafting
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

## Acceptance criteria

- [ ] A test that doesn't set `OAuthBaseURL` cannot accidentally reach
      real GitHub
- [ ] The fixture's docs clearly state the safe-by-default behavior
- [ ] Existing smoke spec continues to work without changes (or with a
      trivial annotation)
