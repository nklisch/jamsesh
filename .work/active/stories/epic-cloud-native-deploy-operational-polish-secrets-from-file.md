---
id: epic-cloud-native-deploy-operational-polish-secrets-from-file
kind: story
stage: done
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

## Implementation notes

### What landed

- **`internal/portal/config/secrets.go`** — new file with unexported `readEnvOrFile(name string) (string, error)` helper. When `name_FILE` is set, reads and returns trimmed file contents (fail-fast on unreadable path). Falls back to plain `os.Getenv(name)`. Returns `("", nil)` when neither is set.

- **`internal/portal/config/config.go`** changes:
  - Package doc comment updated with a "Secret env vars with `_FILE` variants" subsection listing all six new `_FILE` env vars.
  - `applyEnv` signature changed to `func applyEnv(c *Config) error`; `Load` propagates the error.
  - `applyOAuthEnv` signature changed to `func applyOAuthEnv(o *OAuthConfig) error`.
  - `applyEmailEnv` signature changed to `func applyEmailEnv(e *EmailConfig) error`.
  - All six secret-bearing env-overlay sites converted from `os.Getenv` to `readEnvOrFile`.
  - Non-secret vars (Bind, DBDriver, PortalURL, TLS fields, LogFormat, LogLevel, Storage, GitMaxPackBytes) retain plain `os.Getenv` — no `_FILE` support.

- **`internal/portal/config/secrets_test.go`** — 19 test cases in `package config` (internal package, required for testing the unexported helper):
  - `TestReadEnvOrFile_NeitherSet` — neither var set → `("", nil)`
  - `TestReadEnvOrFile_PlainVarSet` — only plain var set → plain value returned
  - `TestReadEnvOrFile_FileWins` — both set → file value wins
  - `TestReadEnvOrFile_FileReadable` — only `_FILE` set → file value returned
  - `TestReadEnvOrFile_TrailingWhitespaceTrimmed` — table-driven: newline, CRLF, spaces+newline, tabs+newline, none, multiple newlines
  - `TestReadEnvOrFile_UnreadablePath` — unreadable path → error mentioning `_FILE` var name
  - One `TestLoad_*File` test per each of the six secret vars (end-to-end through `Load`)
  - `TestLoad_DBDSNFile_UnreadableErrors` — `Load` returns error on unreadable `_FILE`
  - `TestLoad_FilePrecedenceOverPlainVar` — end-to-end precedence check
  - `TestLoad_NonSecretVarsUnaffected` — `JAMSESH_BIND_FILE` has no effect; `Load` does not error

### Deviations from design

None. Implementation matches the feature design exactly. The `readEnvOrFile` signature and behavior match the spec in the story body verbatim.

### Test results

All 35 tests in `internal/portal/config` pass. Full suite (`go test ./...`) passes with all 40 packages green.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The 6-line repetition `v, err := readEnvOrFile("X"); if err != nil {...}; if v != "" { c.X = v }` is verbose but explicit; a one-liner helper would shave lines at the cost of clarity. Skip.
- `strings.TrimRight` is a slight footgun for secrets that legitimately contain trailing whitespace (rare); documented in the helper's doc comment, so operator-aware.

**Notes**: Implementation matches design verbatim. 15 tests cover the full helper state matrix (neither/plain-only/file-only/both/whitespace/unreadable) and end-to-end Load behavior for each of the 6 secret env vars. Package doc comment updated. No foundation-doc drift (SELF_HOST / SPEC updates belong to the sibling docs story).
