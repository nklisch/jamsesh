---
id: epic-e2e-cnd-coverage-cluster-fixture-minio
kind: story
stage: done
tags: [e2e-test, testing, infra]
parent: epic-e2e-cnd-coverage-cluster-fixture
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# MinIO fixture

## Scope

Add a Testcontainers-Go fixture for MinIO under
`tests/e2e/fixtures/minio/`, exposing the same host-side + container-side
endpoint pair pattern as the existing `postgres` fixture. Pre-create a
random-named bucket per test invocation. Ship a `minio_test.go` self-test
that proves a PUT + GET round-trip succeeds via the minio-go SDK.

Image pin: `minio/minio:RELEASE.2024-12-18T13-15-44Z` (stable release,
implementer may bump to a more recent stable tag if needed).

## Files

- `tests/e2e/fixtures/minio/minio.go` — `Start` + `Options` + `MinIO`
  struct with `Endpoint`, `ContainerEndpoint`, `AccessKey`, `SecretKey`,
  `BucketName`.
- `tests/e2e/fixtures/minio/inspect.go` — helpers consumers use:
  `ListObjects(ctx, prefix)`, `GetObject(ctx, key)`,
  `PutObject(ctx, key, data)`, `DeleteObject(ctx, key)`.
- `tests/e2e/fixtures/minio/minio_test.go` — self-test.

## Acceptance criteria

- [x] `minio.Start(ctx, t, minio.Options{})` returns within 30s
- [x] Bucket is pre-created with a random name (`bucket-<hex8>` style;
      hyphens required by S3 naming rules — underscore not permitted)
- [x] Self-test: PUT a small payload, GET it back, assert content equals
- [x] `t.Cleanup` terminates the container
- [x] Test skips cleanly with a clear message when Docker is unavailable
      (existing `requireDocker` pattern)
- [x] `containerlog.DumpAndTerminate` used for cleanup so failure logs
      surface in CI (matches portal/postgres fixtures)
- [x] `go test ./fixtures/minio/...` is green from the `tests/e2e/` module

## Test integrity (from parent epic)

This is fixture code, not test code asserting on product behavior. The
self-test asserts on MinIO's own behavior (PUT then GET should round-trip)
which is a real invariant of the service. Not tautological — proves the
fixture itself works.

If MinIO behaves unexpectedly (e.g., region misconfiguration, path-style
issues), file the discovery as a fixture-bug item and resolve before
marking done. Do not silence by weakening assertions.

## References

- `tests/e2e/fixtures/postgres/postgres.go` — shape to mirror (per-test
  isolation, Container vs host addresses, ContainerIP usage)
- `tests/e2e/fixtures/portal/portal.go` — `requireDocker` + container-log
  cleanup patterns
- Parent feature body for the full unit design

## Implementation notes

### Files touched

- `tests/e2e/fixtures/minio/minio.go` — `Start`, `Options`, `MinIO` struct,
  `randHex`, `requireDocker`.
- `tests/e2e/fixtures/minio/inspect.go` — `ListObjects`, `GetObject`,
  `PutObject`, `DeleteObject` on `*MinIO`.
- `tests/e2e/fixtures/minio/minio_test.go` — `TestMinIOStart` self-test
  (PUT + GET round-trip + `ListObjects`).
- `tests/e2e/go.mod` / `tests/e2e/go.sum` — added `github.com/minio/minio-go/v7`.

### Deviations from design

**Bucket naming:** The design spec used `bucket_<hex8>` (underscore) but
MinIO enforces S3 bucket naming rules (lowercase letters, numbers, hyphens
only — no underscores). The fixture uses `bucket-<hex8>` (hyphen). Discovered
during the first test run: `Bucket name contains invalid characters`.

**Hex length:** `randHex(4)` produces 8 hex characters (4 bytes → 8 chars),
matching the `bucket-<hex8>` naming. The design said "hex8" meaning 8 hex
characters, which is `randHex(4)`. Implementation is correct.

**Wait strategy:** `/minio/health/live` on port 9000 works correctly with the
pinned image `minio/minio:RELEASE.2024-12-18T13-15-44Z`. No fallback needed.

**No shared singleton:** Per design, MinIO is a per-test container (no
`sync.Once`). Each `Start` call gets its own container + random bucket.

**minio-go client construction:** `inspect.go` builds a fresh `miniogo.Client`
per operation call via the private `client()` helper. This is intentional —
the helper keeps the `MinIO` struct lean (no embedded client) and avoids
client lifecycle concerns in fixture teardown.

### Test run

```
--- PASS: TestMinIOStart (1.53s)
PASS
ok  jamsesh/tests/e2e/fixtures/minio  1.564s
```

## Dependencies on this story (downstream)

- `epic-e2e-cnd-coverage-cluster-fixture-portalcluster` (uses MinIO as
  the object-storage backend for clustered portal config)
- Eventually consumed by every test in
  `epic-e2e-cnd-coverage-object-storage-sync`

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Fixture correctly mirrors the portal/postgres patterns. `requireDocker`
skip, `containerlog.DumpAndTerminate` cleanup, `ContainerEndpoint` vs `Endpoint`
split, per-test container isolation — all match spec. Bucket-naming deviation
(underscore → hyphen) is real S3 constraint, documented in implementation notes, and
the story body already called out hyphens as required. Test assertions are
behaviorally meaningful (content equality on GET, key + count on ListObjects). No
tautological assertions. No foundation-doc drift.
