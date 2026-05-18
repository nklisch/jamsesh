---
id: review-file-secret-unreadable-subtest-bug
kind: story
stage: done
tags: [bug, e2e-test, testing]
parent: epic-e2e-cnd-coverage-operational-polish
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Fix `file_unreadable` subtest: host file mode 0o000 breaks testcontainers host-side open

## Bug

In `tests/e2e/failure/file_secret_missing_test.go`, the `file_unreadable`
subtest writes a host-side temp file with mode `0o000`:

```go
require.NoError(t,
    os.WriteFile(secretPath, []byte("ignored"), 0o000),
    "create unreadable secret file")
```

Then passes it as a `ContainerFile` with `HostFilePath: secretPath`.

testcontainers copies container files by calling `os.Open(hostFilePath)` on
the host before the container is created (see
`testcontainers-go@v0.42.0/docker.go:696`). A file with mode `0o000` cannot
be opened by the creating process — `os.Open` returns "permission denied". The
`GenericContainer` call returns an error, and `require.NoError(t, err)` fails.

The test never reaches `assertFileSecretFailure` — it errors out during
container creation, failing for the wrong reason. The portal's behavior on an
unreadable secret file is never actually tested.

## Fix

Write the host file with a readable mode (e.g. `0o600`) so testcontainers can
open and copy it. The `FileMode: 0o000` field on `ContainerFile` controls the
mode *inside the container*, which is what makes it unreadable to the portal
process (nobody):

```go
secretPath := filepath.Join(t.TempDir(), "db_dsn")
require.NoError(t,
    os.WriteFile(secretPath, []byte("ignored"), 0o600), // host-readable
    "create secret file")

// ...
Files: []testcontainers.ContainerFile{
    {
        HostFilePath:      secretPath,
        ContainerFilePath: "/run/secrets/db_dsn",
        FileMode:          0o000, // unreadable inside container — this is what matters
    },
},
```

Alternatively, use `Reader: strings.NewReader("ignored")` to avoid a host
file entirely.

## References

- `tests/e2e/failure/file_secret_missing_test.go` — `file_unreadable` subtest
- `testcontainers-go@v0.42.0/docker.go:696` — `os.Open(hostFilePath)` on host
- `testcontainers-go@v0.42.0/container.go:110-114` — `ContainerFile` struct
  (Reader field is an alternative to HostFilePath)
- Parent story: `epic-e2e-cnd-coverage-operational-polish-file-secrets`

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: The fix (`0o600` host mode, `FileMode: 0o000` on `ContainerFile`) was
applied at commit `a974e2b` and verified during the re-review of the parent story
`epic-e2e-cnd-coverage-operational-polish-file-secrets`. Bug description, root cause,
and fix specification are all accurate.
