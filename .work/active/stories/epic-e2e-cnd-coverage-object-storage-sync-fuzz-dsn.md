---
id: epic-e2e-cnd-coverage-object-storage-sync-fuzz-dsn
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-object-storage-sync
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object Storage — Fuzz: URL Scheme Parser (F10)

Implements `tests/e2e/fuzz/object_storage_dsn_test.go` and
`tests/e2e/fuzz/testdata/object-storage-dsn-corpus.json`.

Addresses audit finding F10 (Medium, missing-taxonomy-layer fuzz): the
object-storage URL parser added by CND (`objectstore.New`, `parseScheme`) has
no fuzz coverage.

## Invariant (property)

Any value of `JAMSESH_OBJECT_STORAGE_URL` either:
- Causes the portal to boot cleanly (URL is valid and backend reachable), OR
- Causes the portal to fail fast at startup with a typed error.

The portal NEVER:
- Panics (nil-deref, SEGV) on any input
- Boots cleanly and then crashes on the first write attempt
- Logs an unhandled error without an exit

## Scope

`TestObjectStorageDSNFuzz`:

- Skip under `-short`.
- For each seed in `testdata/object-storage-dsn-corpus.json`:
  1. Start a portal container via `startFailingPortal` with
     `JAMSESH_DEPLOY_MODE=clustered`, a real Postgres DSN,
     and the seed URL as `JAMSESH_OBJECT_STORAGE_URL`.
  2. Wait up to 15s for the container to exit (fast-fail expected for most seeds).
  3. Property check:
     - If container exited: inspect logs for "panic" or nil-pointer terms.
       Any panic in logs is a production bug.
     - If container is still running: the URL was accepted as valid by the
       parser. This is only expected for well-formed seeds (e.g., a valid
       `s3://bucket/` URL). Assert `/healthz` is reachable to confirm a clean
       boot (not a zombie process).
  4. In neither case should the portal start and then crash on first write —
     this is the "boot-then-crash" anti-pattern.

## Seed corpus: `testdata/object-storage-dsn-corpus.json`

Minimum 15 entries covering:

```json
[
  {"description": "empty string", "url": ""},
  {"description": "no scheme", "url": "bucket/prefix"},
  {"description": "wrong scheme https", "url": "https://bucket/"},
  {"description": "wrong scheme ftp", "url": "ftp://bucket/"},
  {"description": "double scheme", "url": "s3://s3://bucket/"},
  {"description": "s3 no bucket", "url": "s3://"},
  {"description": "s3 empty bucket", "url": "s3:///"},
  {"description": "path traversal", "url": "s3://bucket/../etc/passwd"},
  {"description": "unicode bucket", "url": "s3://bücket/"},
  {"description": "embedded newline", "url": "s3://bucket/\ninjected"},
  {"description": "null byte", "url": "s3://bucket/\x00key"},
  {"description": "percent encoding error", "url": "s3://bucket/%"},
  {"description": "embedded credentials", "url": "s3://user:pass@bucket/"},
  {"description": "overlong 4096 chars", "url": "s3://bucket/AAAA..."},
  {"description": "file scheme", "url": "file:///etc/passwd"},
  {"description": "s3-compatible no endpoint url", "url": "s3-compatible://bucket/"},
  {"description": "valid s3", "url": "s3://valid-bucket-name/prefix/"}
]
```

The last entry ("valid s3") may produce a running container if the backend is
reachable. Other seeds should cause fast-fail.

**Test integrity rules (mandatory for implementer)**:
- A panic in container logs is a production bug. Park it via
  `/agile-workflow:park`, mark that seed's sub-test as `t.Skip` with the
  backlog ID. Do NOT remove the seed from the corpus.
- A "boot-then-crash" (container starts, `/healthz` was once 200, then
  container exits with non-zero on a write attempt) is a production bug.
  Park it the same way.
- Fix stale seeds that no longer exercise the intended path (e.g., if a seed
  URL was made valid by a production change). Update the seed to preserve the
  original intent.

## Acceptance Criteria

- [ ] `TestObjectStorageDSNFuzz` compiles and runs (skip under `-short`)
- [ ] `testdata/object-storage-dsn-corpus.json` has ≥15 entries covering all
      categories above
- [ ] No seed causes a panic (nil-deref, SEGV) in portal logs
- [ ] No seed causes boot-then-crash behavior
- [ ] Any production bugs found are parked with backlog IDs, not silenced
- [ ] No in-process mocks introduced

## Notes

- Each seed starts its own container — this test is intentionally slow and
  must be skipped with `-short`.
- Consider parallelizing seeds via `t.Parallel()` on sub-tests (each seed
  is independent) to reduce wall-clock time.
- The `startFailingPortal` helper from `failure/config_and_deps_test.go`
  may need to be moved to a shared location if used by both `failure/` and
  `fuzz/` packages. Extract to `tests/e2e/fixtures/portalhelper/` if needed.
