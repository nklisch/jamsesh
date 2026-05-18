---
id: epic-e2e-cnd-coverage-object-storage-sync-failure-startup
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

# Object Storage — Failure: Unreachable at Startup

Implements `tests/e2e/failure/object_storage_unreachable_at_startup_test.go`.

## Invariant

A clustered-mode portal configured with an unreachable object-storage URL
exits non-zero at startup. A single-instance portal with the same bad URL
boots normally (the URL is ignored in single mode).

## Scope

`TestObjectStorageUnreachableAtStartup` with two subtests:

- **`clustered_mode_fails_fast`** — start a portal container with:
  - `JAMSESH_DEPLOY_MODE=clustered`
  - `JAMSESH_DB_DRIVER=postgres`, a real Postgres DSN
  - `JAMSESH_OBJECT_STORAGE_URL=s3://unreachable-host-00000/bucket/`
  - `JAMSESH_OBJECT_STORAGE_ENDPOINT_URL=http://unreachable-host-00000:9000`
  - Use the `startFailingPortal` helper pattern from `failure/config_and_deps_test.go`
  - Assert: container exits non-zero within 15s
  - Assert: container logs mention object-storage URL, `obj_store`, or
    `object_storage` (the typed error from config validation or connectivity check)

- **`single_instance_unaffected`** — start a portal via `portal.Start` with:
  - `JAMSESH_DEPLOY_MODE=single` (or omitted — default is single)
  - `JAMSESH_OBJECT_STORAGE_URL=s3://unreachable-host-00000/bucket/` (same bad URL)
  - Assert: `/healthz` returns 200 within 10s
  - Assert: container is still running 5s after the health check passes

**Test integrity rules (mandatory for implementer)**:
- The `single_instance_unaffected` subtest is testing the invariant that
  single-instance mode does NOT require object storage. If the portal actually
  fails on the bad URL in single mode, this is a production bug (the
  config.go validation at line 414 should only enforce the URL for
  `clustered` mode). Park it via `/agile-workflow:park`, skip with backlog ID.
- Do NOT change the assertion to "the portal may or may not fail" — that is a
  tautology.

## Acceptance Criteria

- [ ] `TestObjectStorageUnreachableAtStartup` compiles and runs
- [ ] `clustered_mode_fails_fast`: container exits non-zero within 15s with a
      log line referencing the object-storage configuration
- [ ] `single_instance_unaffected`: portal stays running and `/healthz` returns 200
- [ ] No in-process mocks introduced
- [ ] Any production bugs are parked, not silenced

## Notes

- Use `postgres.Start` for the Postgres backend in the clustered subtest — a
  real Postgres is required since `JAMSESH_DEPLOY_MODE=clustered` also requires
  `JAMSESH_DB_DRIVER=postgres`.
- The `startFailingPortal` helper (from `failure/config_and_deps_test.go`)
  starts a container without a health-check wait and polls for exit. Reuse
  that pattern rather than duplicating it — move it to a shared test helper
  file if both test files need it.
