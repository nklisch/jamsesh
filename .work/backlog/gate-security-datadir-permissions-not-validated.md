---
id: gate-security-datadir-permissions-not-validated
kind: story
stage: drafting
tags: [security, plugin, hardening]
parent: null
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-25
updated: 2026-05-25
---

# DataDir() silently reuses existing dir without validating permissions

## Severity
Low

## Domain
Secrets & Configuration

## Location
`cmd/jamsesh/state/state.go:30-53`

## Evidence
```go
if d := os.Getenv("JAMSESH_DATA_DIR"); d != "" {
    if err := os.MkdirAll(d, 0o700); err != nil { ... }
    return d, nil
}
...
dir := filepath.Join(base, "jamsesh")
if err := os.MkdirAll(dir, 0o700); err != nil { ... }
return dir, nil
```

## Remediation direction
`os.MkdirAll` only sets mode 0700 on directories it creates — pre-existing
directories keep their mode. Tokens, refresh tokens, and per-session
bearers are written underneath this directory. After resolving the path,
`os.Stat` and refuse to use it (or chmod 0700) if the mode is permissive
(group/world read or write). Especially important for the new
`JAMSESH_DATA_DIR` path which is now operator-settable; prior to this
bundle the env var was required and the dir was pre-created by the CC
runtime; the new XDG self-default extends the surface to default-XDG paths
an attacker could pre-create with loose perms.
