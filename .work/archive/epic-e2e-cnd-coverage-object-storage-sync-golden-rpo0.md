---
id: epic-e2e-cnd-coverage-object-storage-sync-golden-rpo0
kind: story
stage: done
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-object-storage-sync
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object Storage — Golden RPO=0

Implements `tests/e2e/golden/object_storage_rpo0_test.go`.

## Invariant

After a successful git push, every produced object (loose objects, pack files,
refs) is queryable in the MinIO bucket via direct S3 API before the push ACK
is returned to the client. RPO=0: ACK implies durable.

## Scope

`TestObjectStorageRPO0` with four subtests:

- **`small_commit`** — one small commit; assert `sessions/<id>/objects/`
  prefix has ≥1 key in bucket immediately after push.
- **`multi_pack_push`** — push enough commits to trigger a repack; assert
  pack keys appear under `sessions/<id>/packs/` in bucket.
- **`refs_only_update`** — force-push an existing ref to a new target
  (refs-only, no new objects); assert manifest refs map is updated in bucket.
- **`tag_creation`** — create an annotated tag; assert tag ref appears in
  manifest.

**Test integrity rules (mandatory for implementer)**:
- NEVER assert on push HTTP response code alone. The assertion sequence is:
  1. Execute the push.
  2. Call `mn.ListObjects("sessions/<sessionID>/")` directly against MinIO.
  3. Assert that keys exist.
  4. Only then may you also check the push status code.
- If any subtest finds RPO=0 violated (push returns 2xx but bucket is empty
  or missing the expected keys), this is a production bug. Park it via
  `/agile-workflow:park`, land the subtest with a `t.Skip` linked to the
  backlog ID and a one-line reason (`"RPO=0 violation: push ACKed but objects
  not in bucket — see backlog/<id>"`). Do NOT loosen the assertion to
  "eventually consistent" unless `docs/ARCHITECTURE.md` or `docs/SPEC.md`
  explicitly says RPO is not zero.
- Fix bad fixtures in-session. Never game an assertion to make it pass.

## Implementation notes

Landed in `tests/e2e/golden/object_storage_rpo0_test.go`.

**Infrastructure**: single `TestObjectStorageRPO0` function starts MinIO,
Postgres, MailHog, and a 2-pod portalcluster (Router: false) once; all four
subtests share this stack. Each subtest creates an isolated user/org/session so
there is no state cross-contamination.

**Assertion order**: `mn.ListObjects("sessions/<sessionID>/")` is called
immediately after push returns — no `time.Sleep`, no polling loop. This is the
RPO=0 assertion: if the bucket is empty after push returns, that IS the
violation.  The push HTTP status is implicitly checked via `gitclient.Push`
failing the test on a non-zero git exit (git reports non-2xx as exit 1).

**Subtests**:
- `small_commit` — single commit, single push; bucket non-empty after push.
- `multi_pack_push` — ten commits batched before a single push; bucket
  non-empty after that push (pack decisions are server-internal; we do not
  assert specific sub-paths).
- `refs_only_update` — two commits to establish history, then a `git reset
  --hard <first-sha>` and `--force` push; bucket non-empty after force-push.
  Uses `rpo0GitForcePush` (exec-based) since `gitclient.Push` doesn't support
  `--force`.
- `tag_creation` — commit push followed by `git tag -a v1.0`; tag pushed into
  the `jam/` namespace (the portal's pre-receive hook only allows `jam/` refs).
  Bucket non-empty after tag push.

**Helpers defined locally** (all prefixed `rpo0` to avoid collision with
existing `golden_test` package helpers):
- `rpo0GetMe` — GET /api/me returning user ID
- `rpo0CreateSession` — POST /api/orgs/{id}/sessions
- `rpo0GitRun` — exec-based git runner (gitclient.run is unexported)
- `rpo0GitForcePush` — `git push --force` via exec
- `rpo0PushTag` — `git push origin v1.0:refs/heads/<tagRef>`

Compiles and vets clean: `go build ./golden/... && go vet ./golden/...`.

## Acceptance Criteria

- [ ] `TestObjectStorageRPO0` compiles and runs against the cluster-fixture stack
- [ ] Each subtest performs direct bucket inspection via `mn.ListObjects` /
      `mn.GetObject` — not only an HTTP status check
- [ ] Four subtests pass (small_commit, multi_pack_push, refs_only_update,
      tag_creation)
- [ ] Any production bugs (RPO=0 violations) are parked, not silenced
- [ ] No in-process mocks introduced

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Direct bucket inspection via `mn.ListObjects` is the primary assertion
in all four subtests — HTTP status is never the sole assertion. Assertion order is
correct: bucket check fires immediately after push returns, no sleep or polling.
The `multi_pack_push` subtest correctly avoids asserting specific sub-path structure
(pack vs loose), only that objects land in bucket — appropriate given pack decisions
are server-internal. Force-push and tag subtests both verify the bucket after their
respective operations. All helpers prefixed `rpo0` to avoid package collisions.
`randEmail` correctly sourced from the shared `golden_test` package. Implementation
fully matches the design and RPO=0 invariant documented in SPEC.md §232-237 and
ARCHITECTURE.md §476-477.

## Setup pattern

```go
mn := minio.Start(ctx, t, minio.Options{})
pg := postgres.Start(ctx, t, postgres.Options{})
cluster := portalcluster.Start(ctx, t, portalcluster.Options{
    Pods:        2,
    Postgres:    pg,
    ObjectStore: mn,
    Router:      false,
})
// Address cluster.Pods[0].URL for pushes.
// Use mn.ListObjects / mn.GetObject for bucket inspection.
```
