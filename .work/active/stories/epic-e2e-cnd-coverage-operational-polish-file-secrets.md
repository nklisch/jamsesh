---
id: epic-e2e-cnd-coverage-operational-polish-file-secrets
kind: story
stage: done
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

## Review (2026-05-17)

**Verdict**: Request changes

**Blockers**: `review-file-secret-unreadable-subtest-bug`
**Important**: none
**Nits**: none

**Notes**: The `ContainerFiles` extension to `portal.Options` is correct and
backward-compatible. The `file_missing` subtest and golden happy-path test are
both correct. The blocker is in the `file_unreadable` subtest:
`os.WriteFile(secretPath, []byte("ignored"), 0o000)` creates a host-side file
that the test process itself cannot read. testcontainers copies container files
by calling `os.Open(hostFilePath)` on the host before container creation —
this returns "permission denied", causing `GenericContainer` to error before
the portal ever runs. The test fails for the wrong reason and the portal's
behavior on an unreadable secret file is never verified. Fix: write the host
file with `0o600` (so testcontainers can open it) and let `FileMode: 0o000`
on the `ContainerFile` struct set the unreadable permission inside the
container (that field controls the in-container mode, not the host mode).

## Review (2026-05-17) — re-review after fix

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Fix at `a974e2b` is correct. The `file_unreadable` subtest now
writes the host file with `0o600` so testcontainers can open it during
container build, and preserves `FileMode: 0o000` on the `ContainerFile`
struct to enforce unreadability inside the container. That is exactly the
right split: host mode for testcontainers, in-container mode for the test
contract. The `file_missing` subtest, golden happy-path test, `ContainerFiles`
fixture extension, and backward compatibility are all unchanged and correct.
No further findings.

## Review findings

### Blockers

- **`file_unreadable` subtest: host file mode 0o000 breaks testcontainers
  host-side open** — `os.WriteFile` with `0o000` makes the file unreadable
  on the host; testcontainers calls `os.Open(hostFilePath)` and fails before
  the container starts. The portal contract is never exercised.
  → Item: `review-file-secret-unreadable-subtest-bug`

## Implementation notes

### Fixture extension (`tests/e2e/fixtures/portal/portal.go`)

Added `ContainerFiles []testcontainers.ContainerFile` to `Options` after
`ExtraEnv`. Wired via `Files: opts.ContainerFiles` in the
`ContainerRequest`. Zero-value `nil` produces no mounts — full backward
compatibility with all existing callers confirmed via `go build ./...`.

### Happy-path test (`tests/e2e/golden/file_secret_happy_path_test.go`)

`TestFileSecretHappyPath` starts a Postgres fixture, writes `pg.ContainerDSN`
to a host-side temp file, and mounts it into the portal at
`/run/secrets/db_dsn`. `ExtraEnv` sets both `JAMSESH_DB_DSN_FILE` (the
mounted path) and clears `JAMSESH_DB_DSN` to `""` (overriding `buildEnv`'s
`:memory:` default). `readEnvOrFile` gives `_FILE` precedence over the plain
var, so either approach works — clearing the plain var makes the test intent
unambiguous. `portal.Start`'s `/healthz` wait strategy proves the portal
fully booted with the Postgres DSN from the secret file.

No extra `OmitDBDSN bool` flag was needed: `ExtraEnv` runs last in `buildEnv`
and overwrites existing keys (verified at `portal.go:218-221`), so
`ExtraEnv["JAMSESH_DB_DSN"] = ""` reliably clears the default.

### Failure tests (`tests/e2e/failure/file_secret_missing_test.go`)

`TestFileSecretMissing` reuses the `requireDockerLocal` / `requirePortalImageLocal`
helpers from `config_and_deps_test.go` (same package). Two subtests:

- `file_missing`: `JAMSESH_DB_DSN_FILE=/no/such/file` — no mount, file never exists.
- `file_unreadable`: file mounted with `FileMode: 0o000`; container process
  (nobody) cannot open it.

Both use raw `testcontainers.GenericContainer` (no `WaitingFor`) — the same
pattern as `startFailingPortal`. `assertFileSecretFailure` polls for
`status == "exited"` within 30 s, asserts exit code ≠ 0, then reads logs
and requires a substring matching `_file` (lower-cased), `"read secret"`, or
`"secret file"`. `config/secrets.go:readEnvOrFile` produces
`"config: read JAMSESH_DB_DSN_FILE (<path>): <os error>"` — always contains
`_FILE` — satisfying the assertion.

### Deviations from story spec

None material. The story spec suggested `io.ReadAll` without an import — added
`"io"` to the failure test imports. The happy-path spec suggested passing
`DBDriver: "postgres"` with an empty `DBDSN`; implemented exactly that.
