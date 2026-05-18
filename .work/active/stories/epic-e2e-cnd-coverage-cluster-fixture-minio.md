---
id: epic-e2e-cnd-coverage-cluster-fixture-minio
kind: story
stage: implementing
tags: [e2e-test, testing, infra]
parent: epic-e2e-cnd-coverage-cluster-fixture
depends_on: []
release_binding: null
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

- [ ] `minio.Start(ctx, t, minio.Options{})` returns within 30s
- [ ] Bucket is pre-created with a random name (`bucket_<hex8>` style,
      matching postgres-fixture's `test_<hex8>` DB naming)
- [ ] Self-test: PUT a small payload, GET it back, assert content equals
- [ ] `t.Cleanup` terminates the container
- [ ] Test skips cleanly with a clear message when Docker is unavailable
      (existing `requireDocker` pattern)
- [ ] `containerlog.DumpAndTerminate` used for cleanup so failure logs
      surface in CI (matches portal/postgres fixtures)
- [ ] `go test ./fixtures/minio/...` is green from the `tests/e2e/` module

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

## Dependencies on this story (downstream)

- `epic-e2e-cnd-coverage-cluster-fixture-portalcluster` (uses MinIO as
  the object-storage backend for clustered portal config)
- Eventually consumed by every test in
  `epic-e2e-cnd-coverage-object-storage-sync`
