---
id: gate-security-git-extraheader-argv-credential-leak
kind: story
stage: implementing
tags: [security, cli]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: security
created: 2026-05-31
updated: 2026-05-31
---

# Git bearer credentials are exposed through process arguments

## Severity
High

## Domain
Secrets & Configuration

## Location
`cmd/jamsesh/sessioncmd/join.go:136`

## Evidence
```go
basicHeader := "Authorization: Basic " + base64.StdEncoding.EncodeToString(
	[]byte("x-access-token:"+tok))
localPath := sessionID + ".git"
if err := runGitWithEnv(
	nil,
```

Related push paths in `cmd/jamsesh/sessioncmd/new.go` also pass bearer material
through `git -c http.extraHeader=...`.

## Remediation direction
Stop passing `Authorization` headers through process arguments. Use a scoped
credential helper, askpass flow, or protected temporary config/helper file so
bearer material is not visible in argv or persisted git config.

