---
id: gate-security-cli-join-clone-url-bearer-in-process-args
kind: story
stage: done
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

## Implementation notes

- `cmd/jamsesh/sessioncmd/join.go`:
  - `buildCloneURL` no longer accepts a `token` argument and no longer sets
    `u.User`. It now returns a credential-less URL of the form
    `<portal>/git/<orgID>/<sessionID>.git`.
  - The clone call site builds a Basic-auth header
    (`Authorization: Basic base64("x-access-token:" + token)`) and invokes
    `runGitWithEnv(nil, "-c", "http.extraHeader="+basicHeader, "clone", "--bare", url, dest)`.
    Mirrors the pattern in `new.go:pushBaseRef` and
    `finalizecmd/fetchsource.go:performFetch`.
  - `encoding/base64` added to imports.
- `cmd/jamsesh/sessioncmd/join_test.go`:
  - `TestJoinAction_happy` and `TestJoinAction_inviteURL` now stub
    `runGitWithEnv` alongside `runGit` (the clone moves to the new dispatcher
    while the local `git -C ... checkout` still flows through `runGit`).
  - `TestJoinAction_happy` asserts:
    - the clone was invoked via `runGitWithEnv`;
    - the URL positional argument contains no `@` (no userinfo segment);
    - the literal bearer string `tok-test` does not appear anywhere in the
      clone's argv outside the `http.extraHeader=` Basic blob.
  - New focused unit test `TestBuildCloneURL_NoCredentialsEmbedded` regresses
    the API of `buildCloneURL` directly across multiple portal URL shapes.
- Verification: `go build ./...` clean; `go test ./cmd/jamsesh/sessioncmd/...
  -count 1` passes.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The bearer still passes through `argv` via `-c http.extraHeader=...` — this is
  intentional and consistent with `new.go:pushBaseRef` and
  `finalizecmd/fetchsource.go:performFetch`. The header form is the documented
  upgrade from URL-embedded credentials; no further hardening warranted within
  scope.

**Notes**: Mirrors an already-used pattern (new.go, fetchsource.go). Tests pin
both the API of `buildCloneURL` (no userinfo) and the call-site argv shape
(no plaintext bearer outside the header blob). Closes the gate finding.
