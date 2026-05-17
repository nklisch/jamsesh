---
id: epic-e2e-tests-infrastructure-portal-oauth-base-url
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-tests-infrastructure
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Infra — Portal OAuth base-URL config wiring

## Scope

Wire `JAMSESH_OAUTH_GITHUB_BASE_URL` (and the YAML field
`oauth.github.base_url`) into `portaloauth.GitHubOptions.BaseURL` so
e2e tests can substitute WireMock for github.com in a black-box
configuration. Touches portal code, not test code.

## Background

`internal/portal/oauth/github.go` already exposes
`GitHubOptions.BaseURL` for test substitution. The unit tests use it
via direct construction in `internal/portal/oauth/github_test.go`.
The portal binary's config struct
(`internal/portal/config/config.go > GitHubOAuthConfig`) does NOT
expose this field — only `ClientID` and `ClientSecret` flow through.
E2E tests need black-box config-driven substitution.

## Files to create / modify

- `internal/portal/config/config.go`:
  - Add `BaseURL string` field to `GitHubOAuthConfig` with
    `yaml:"base_url"`
  - Add `JAMSESH_OAUTH_GITHUB_BASE_URL` to `applyOAuthEnv`
  - Add the env var name to the package doc-comment env-var list
- `cmd/portal/main.go`:
  - When building the GitHub provider, pass
    `BaseURL: cfg.OAuth.GitHub.BaseURL`
- `internal/portal/config/config_test.go`:
  - New subtest verifying the env var flows through
- `internal/portal/oauth/github_test.go`:
  - New subtest (or extend existing) verifying that when
    `GitHubOptions.BaseURL` is set and the provider's outbound calls
    are intercepted via `httptest.Server`, all three GitHub URLs
    (`/login/oauth/access_token`, `/user`, `/user/emails`) are
    rewritten correctly
- `docs/SELF_HOST.md`:
  - Add the new env var to the configuration table (rolling-forward
    foundation-doc principle)

## Acceptance criteria

- [ ] `go test ./internal/portal/config/...` green; new subtest
      exercises the env var
- [ ] `go test ./internal/portal/oauth/...` green; new subtest
      verifies base-URL substitution for all three GitHub endpoints
- [ ] `docs/SELF_HOST.md` configuration table includes the new env
      var with description "Override GitHub OAuth base URL for
      testing; leave unset in production"
- [ ] Setting the env var to a fake URL and starting the portal
      results in OAuth-start requests going to the fake URL (proven
      by the test, not just code inspection)

## Notes for the implementer

- The env var is for testing only — the production guidance in the
  docs should be explicit ("Leave unset in production")
- The OAuth provider's `BaseURL` is used for both `github.com` (login
  + token exchange) and `api.github.com` (user/email). Verify the
  test exercises both substitution paths
- The existing config struct is YAML-driven; follow the existing
  pattern in `applyEnv` / `applyOAuthEnv` for consistency
