---
id: gate-security-cli-join-clone-url-bearer-in-process-args
kind: story
stage: implementing
tags: [security, plugin, cli, auth]
parent: null
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-25
updated: 2026-05-25
---

# CLI join clone URL embeds bearer in process args (visible via `ps`)

## Severity
Medium

## Domain
Secrets & Configuration / Error Handling

## Location
`cmd/jamsesh/sessioncmd/join.go:213-222`

## Evidence
```go
func buildCloneURL(portalURL, token, orgID, sessionID string) string {
    u, err := url.Parse(portalURL)
    ...
    u.User = url.UserPassword("x-access-token", token)
    ...
    return u.String()
}
// call site:
cloneURL := buildCloneURL(portalURL, tok, orgID, sessionID)
if err := runGit("clone", "--bare", cloneURL, localPath); err != nil {
```

## Remediation direction
Mirror the pattern documented in `cmd/jamsesh/sessioncmd/new.go:262-298` and
`cmd/jamsesh/finalizecmd/fetchsource.go:111-112` — pass the bearer via
`-c http.extraHeader="Authorization: Basic <b64>"` to `git clone` and pass a
credential-less remote URL. The current join.go form leaves the bearer
visible to any other local process via `ps aux` for the duration of the
clone, and may surface in any git failure output the OS pipes to a parent
log. The same file's `pushBaseRef` already chose `extraHeader` and called
out URL-credentials as a leak vector in its docstring — `joinSession` should
do the same.
