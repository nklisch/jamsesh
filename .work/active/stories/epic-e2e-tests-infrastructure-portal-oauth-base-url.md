---
id: epic-e2e-tests-infrastructure-portal-oauth-base-url
kind: story
stage: done
tags: [e2e-test, testing, portal]
parent: epic-e2e-tests-infrastructure
depends_on: []
release_binding: v0.1.0
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

## Implementation notes

Files modified:

- `internal/portal/config/config.go`: added `BaseURL string` field to `GitHubOAuthConfig` with `yaml:"base_url"`, added `JAMSESH_OAUTH_GITHUB_BASE_URL` env var to `applyOAuthEnv`, added both YAML key and env var to package doc comment
- `cmd/portal/main.go`: added `BaseURL: cfg.OAuth.GitHub.BaseURL` to `GitHubOptions` struct literal when constructing the GitHub provider
- `internal/portal/config/config_test.go`: added `TestOAuthGitHubBaseURLEnvOverride` subtest; also added `JAMSESH_OAUTH_GITHUB_CLIENT_ID`, `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET`, and `JAMSESH_OAUTH_GITHUB_BASE_URL` to `clearEnv` helper for isolation
- `internal/portal/oauth/github_test.go`: added `TestGitHub_BaseURL_SubstitutesAllEndpoints` with a `recordingTransport` that asserts all three endpoint paths (`/login/oauth/access_token`, `/user`, `/user/emails`) are hit on the fake server and no requests escape to real github.com
- `docs/SELF_HOST.md`: removed stale placeholder NOTE about OAuth env vars landing in a future release; added rows for `JAMSESH_OAUTH_GITHUB_CLIENT_ID`, `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET`, and `JAMSESH_OAUTH_GITHUB_BASE_URL` to the configuration reference table (rolling-forward principle — these env vars are already implemented)

Deviations from story spec: the SELF_HOST.md change adds three rows instead of one. The stale NOTE claimed OAuth vars were not yet implemented, which is false; removing it and documenting all three active OAuth env vars is the correct rolling-forward update. The acceptance criterion for the BASE_URL row is satisfied.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The `BaseURL` field comment "Leave empty in production" could be slightly more emphatic about the test-vs-production boundary. Optional polish.

**Notes**: The diff is surgical (1-line `main.go` wiring + 1 field + 1 env-overlay branch + 2 focused tests + accurate SELF_HOST.md update). Both tests cover meaningful behavior — `TestOAuthGitHubBaseURLEnvOverride` proves env → config; `TestGitHub_BaseURL_SubstitutesAllEndpoints` proves config → all three GitHub endpoints route through the substituted host with a recording transport that fails if any request escapes. Removing the stale NOTE is correct rolling-forward.
