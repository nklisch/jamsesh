---
id: epic-cloud-native-deploy-operational-polish-secrets-from-file
kind: story
stage: implementing
tags: [infra, portal]
parent: epic-cloud-native-deploy-operational-polish
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Operational Polish — `_FILE` secret env-var variants

## Scope

Add a `_FILE` companion to every secret-bearing env var. When the
`_FILE` variant is set, the value is read from the file at that path
(trailing whitespace trimmed). `_FILE` takes precedence over the
plain var when both are set. Failure to read a configured `_FILE` is
fail-fast at startup.

Implements **Unit 3** of `epic-cloud-native-deploy-operational-polish`.

## Files

New:
- `internal/portal/config/secrets.go` — `readEnvOrFile` helper
- `internal/portal/config/secrets_test.go`

Edit:
- `internal/portal/config/config.go` — refactor every secret-bearing
  env-overlay site (DB DSN, OAuth client secret, SMTP password,
  SendGrid API key, Postmark server token, Resend API key) to call
  `readEnvOrFile`; change `applyEnv` and helpers to return error;
  propagate up through `Load`
- Top-of-file doc comment listing the new `_FILE` variants

## Interface

```go
// internal/portal/config/secrets.go
package config

// readEnvOrFile returns the value for env var `name`, preferring
// the contents of the file named by `name + "_FILE"` when that var
// is set. Trailing whitespace (including newlines) is trimmed.
// Returns ("", nil) when neither variable is set.
// Returns ("", err) when `_FILE` is set but unreadable.
func readEnvOrFile(name string) (string, error)
```

New env vars:
- `JAMSESH_DB_DSN_FILE`
- `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE`
- `JAMSESH_EMAIL_SMTP_PASS_FILE`
- `JAMSESH_EMAIL_SENDGRID_API_KEY_FILE`
- `JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN_FILE`
- `JAMSESH_EMAIL_RESEND_API_KEY_FILE`

## Acceptance criteria

- [ ] `_FILE` set + readable → value is file contents (trimmed).
- [ ] `_FILE` set + unreadable → `Load` returns error.
- [ ] Both `_FILE` and plain var set → file value wins.
- [ ] Neither set → empty string, no error.
- [ ] Trailing newline / whitespace stripped from file contents.
- [ ] Non-secret env vars (Bind, PortalURL, LogLevel, etc.) still
  use plain `os.Getenv` — `_FILE` is a secrets-only convention.
- [ ] Package doc comment lists every new `_FILE` variant.
- [ ] Unit tests cover all four state combinations + unreadable
  path + trailing-whitespace stripping.
