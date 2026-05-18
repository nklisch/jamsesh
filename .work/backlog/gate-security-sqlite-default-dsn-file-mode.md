---
id: gate-security-sqlite-default-dsn-file-mode
kind: story
stage: backlog
tags: [security, portal, documentation]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# SQLite default DSN path is relative (`./jamsesh.db`) — file mode defaults to 0644

## Severity
Low

## Domain
Secrets & Configuration

## Location
`internal/portal/config/config.go:350`

## Evidence
```go
DBDSN:     "./jamsesh.db",
```

The portal binary runs as `USER nobody` in the Dockerfile, but
`modernc.org/sqlite` creates the DB file with the default umask
(typically 0644) — readable by every container user. In the same-host
self-host model (e.g. running `portal` directly), `./jamsesh.db` may
land in the user's working directory readable by other local users. The
DB holds all oauth_tokens, magic_link_tokens, and account email PII.

## Remediation direction
Document that the operator must chmod the SQLite file to 0600 after
first start, or chmod the parent directory to 0700. Optionally wrap the
`db.Open` path with a post-open `os.Chmod(path, 0600)` for sqlite DSNs.
