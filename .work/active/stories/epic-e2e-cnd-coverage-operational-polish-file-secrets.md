---
id: epic-e2e-cnd-coverage-operational-polish-file-secrets
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-operational-polish
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# `_FILE` secret env-var coverage — golden + failure

## Scope

Two test files exercising the `_FILE` secret indirection. Verified env
vars from `internal/portal/config/config.go:57-64`:
`JAMSESH_DB_DSN_FILE`, `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE`,
`JAMSESH_EMAIL_SMTP_PASS_FILE`, `JAMSESH_EMAIL_SENDGRID_API_KEY_FILE`,
`JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN_FILE`,
`JAMSESH_EMAIL_RESEND_API_KEY_FILE`. File contents take precedence;
loader errors if `_FILE` is set but unreadable.

Tests focus on `JAMSESH_DB_DSN_FILE` as the canonical example. Pattern
extends to others; explicit per-variant tests are out of scope here
(adding coverage for the other 5 `_FILE` vars is a single-stride
follow-up if desired).

## Files

- `tests/e2e/golden/file_secret_happy_path_test.go`
- `tests/e2e/failure/file_secret_missing_test.go`
- `tests/e2e/fixtures/portal/portal.go` (extension — add
  `ContainerFiles []testcontainers.ContainerFile` to `Options`)

## Acceptance criteria

- [ ] `portal.Options` gains a `ContainerFiles
      []testcontainers.ContainerFile` field, wired into the underlying
      `testcontainers.ContainerRequest.Files`
- [ ] Golden: portal boots with `JAMSESH_DB_DSN_FILE=/run/secrets/db_dsn`
      pointing at a mounted file containing the Postgres DSN; `/healthz`
      returns 200 (proven by `portal.Start`'s wait strategy)
- [ ] Failure `file_missing`: portal exits non-zero within 30s when
      `JAMSESH_DB_DSN_FILE` points at `/no/such/file`; container logs
      contain a substring matching `_FILE` (case-insensitive) or
      `"read secret"`
- [ ] Failure `file_unreadable`: file is mounted with mode 0o000 (or
      owned by root with portal running as `nobody`); same assertions
      as `file_missing`
- [ ] No silent fallback to env-var-only — if portal silently uses
      an empty DSN, the test catches it
- [ ] `portal.Options.ContainerFiles` change is backward-compatible
      (existing tests don't change behavior)

## Test integrity (from parent epic)

- A test that "passes" because the portal silently accepts a missing
  `_FILE` and uses a default empty DSN is worse than no test. Assertion
  must verify exit code AND log content.
- If portal hangs instead of exiting on `_FILE` failure (no timeout on
  startup file-read), the 30s assertion bound catches it — that's a real
  bug. Park, t.Skip with reference.

## References

- Parent feature body, Unit 3 — full scaffold + `portal.Options`
  extension note
- `internal/portal/config/config.go:57-64,448,601,620` — `_FILE` env
  var list + error path
- `tests/e2e/failure/config_and_deps_test.go:296` — existing
  `ContainerFile` mount pattern
- `tests/e2e/fixtures/portal/portal.go:45-79` — `Options` struct to extend
