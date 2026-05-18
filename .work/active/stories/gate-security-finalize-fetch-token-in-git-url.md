---
id: gate-security-finalize-fetch-token-in-git-url
kind: story
stage: backlog
tags: [security, portal, plugin]
parent: null
depends_on: []
release_binding: null
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
