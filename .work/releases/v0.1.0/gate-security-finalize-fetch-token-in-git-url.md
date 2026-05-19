---
id: gate-security-finalize-fetch-token-in-git-url
kind: story
stage: done
tags: [security, portal, plugin]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Finalize fetch token spliced into git remote URL `https://x-access-token:TOKEN@.../`

## Severity
Low

## Domain
Data Protection

## Location
`internal/portal/finalize/fetch_token.go:94-105`

## Evidence
```go
u.User = url.UserPassword("x-access-token", rawToken)
u.Path = fmt.Sprintf("/git/%s/%s.git", orgID, sessionID)
return u.String(), nil
```

The composed URL is returned in the API response and consumed by
`cmd/jamsesh/finalizecmd/fetchsource.go`. Tokens-in-URL hit local git
config (`.git/config` after a clone), shell history, and `ps -ef` output
for the duration of the git command. TTL is 5 minutes which limits
exposure but does not eliminate it.

## Remediation direction
Return the token separately and have the plugin pass it via
`git credential helper` or
`git -c http.extraHeader="Authorization: Bearer …"` rather than splicing
it into the remote URL. At minimum, document that the URL must never be
persisted into a remote config.

## Implementation notes

### Files changed

- **`docs/openapi.yaml`** — `FetchTokenResponse` schema updated: `remote_url`
  description changed from "pre-composed URL with token spliced into userinfo"
  to "plain git remote URL with no credentials". The token field description
  updated to say "pass via `Authorization: Bearer` via `git -c
  http.extraHeader`". Codegen run: `make generate-api-go`.

- **`internal/api/openapi/server.gen.go`** — regenerated; `FetchTokenResponse`
  comment now says "Plain git remote URL with no credentials".

- **`internal/portal/finalize/fetch_token.go`** — `composeFetchRemoteURL`
  signature dropped the `rawToken` param; it no longer calls `url.UserPassword`
  or sets `u.User`. The handler comment updated to reflect the new credential
  strategy. The `IssueFetchToken` call site updated accordingly.

- **`internal/portal/finalize/fetch_token_test.go`** — Two tests renamed and
  rewritten: `TestIssueFetchToken_RemoteURLCarriesTokenInUserinfo_HTTPS` →
  `TestIssueFetchToken_RemoteURLIsPlainNoCredentials_HTTPS` and the HTTP
  variant likewise. Both now assert `u.User == nil` and that the Token field
  is non-empty separately.

- **`cmd/jamsesh/finalizecmd/fetchsource.go`** — `fetchSource` struct gained a
  `Token string` field. `chooseFetchSource` validates the token is non-empty
  and stores it; `git remote add jamsesh <url>` now uses the plain URL.
  `performFetch` for the https case now calls
  `runGitVerbose(out, "-c", "http.extraHeader=Authorization: Bearer "+fs.Token, "fetch", jamseshRemoteName)`.

- **`cmd/jamsesh/finalizecmd/fetchsource_test.go`** — Mock responses updated
  to return plain URLs. Assertions updated to check no credentials in URL and
  that `fs.Token` is populated. `TestPerformFetch_HTTPSKindUsesRemoteName`
  renamed to `TestPerformFetch_HTTPSKindPassesTokenViaExtraHeader`; asserts
  the `-c http.extraHeader=...` args are present.

- **`cmd/jamsesh/finalizecmd/finalizerun_cleanup_test.go`** — SIGINT simulation
  stub updated to detect `fetch jamsesh` at any position in args (previously
  matched `args[0] == "fetch"` which broke when `-c` was prepended).

- **`tests/e2e/fixtures/checkout/checkout.go`** — `RunPlan` signature gained
  a `fetchToken string` parameter. When non-empty, injects
  `GIT_CONFIG_COUNT=1 / GIT_CONFIG_KEY_0=http.extraHeader /
  GIT_CONFIG_VALUE_0=Authorization: Bearer <token>` into the script's
  environment so `git fetch "$JAMSESH_FETCH_REMOTE"` can authenticate
  against the live portal without credentials in the URL.

- **`tests/e2e/golden/finalize_plan_test.go`** — Updated `RunPlan` call to
  pass `fetchTok.Token` as the third argument. Added assertion that
  `fetchTok.Token` is non-empty.

- **`docs/SECURITY.md`** — Added a bullet in the Self-host security posture
  section documenting that finalize fetch tokens are passed via
  `Authorization: Bearer` header (using `git -c http.extraHeader`), not
  embedded in git remote URLs. Explains the impact on `.git/config` persistence
  and proxy access log scoping.

### git argv shape for HTTPS fetch

```
git  -c  http.extraHeader=Authorization: Bearer <token>  fetch  jamsesh
```

This is passed as a slice of separate argv elements (no shell involvement):
`[]string{"-c", "http.extraHeader=Authorization: Bearer <token>", "fetch", "jamsesh"}`.
The `-c` flag sets a one-shot config value for the duration of the subprocess;
it is not persisted to `.git/config`.

### Codegen confirmation

`make generate-api-go` ran successfully and updated `server.gen.go` with the
revised description for `FetchTokenResponse.RemoteUrl`.

## Review (2026-05-18)

**Verdict**: Approve with comments

**Blockers**: none
**Important**:
- **Token echoed to stdout via `runGitVerbose`** (`cmd/jamsesh/finalizecmd/execute.go:116`):
  the verbose command-line print includes `-c http.extraHeader=Authorization: Bearer <token>`
  verbatim. This is a new (smaller) leak surface that the original spec didn't
  cover — the threats it addressed (`.git/config`, `ps -ef`, shell history) are
  successfully closed, but terminal scrollback and CI captures now see the token.
  → Item: `finalize-fetch-token-leak-via-rungitverbose-echo`

**Nits**: none.

**Notes**: The primary spec is met cleanly — server returns plain URL + separate
token field, plugin passes `-c http.extraHeader`, OpenAPI/codegen updated,
e2e fixture updated, docs updated, both `_HTTPS` and `_HTTP` server tests
rewritten with the new shape. The follow-up item targets the verbose-echo leak
specifically rather than re-opening the whole change.
