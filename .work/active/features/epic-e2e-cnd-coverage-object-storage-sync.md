---
id: epic-e2e-cnd-coverage-object-storage-sync
kind: feature
stage: drafting
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E CND Coverage — Object-Storage Sync

## Brief

`epic-cloud-native-deploy-object-storage-sync` shipped the durability
layer for clustered mode: continuous mirror of bare-repo writes to S3 /
GCS / Azure / S3-compatible. Object storage is the system of record in
clustered mode; local disk is a working cache.

Coverage today: zero. No MinIO / LocalStack / GCS-emulator / Azurite
fixture exists. No test references `s3://`, `gs://`, `azblob://`,
`s3-compatible://`, RPO, pack manifest, or `gc.auto`.

This is the **durability surface**. The RPO=0 contract — every
acknowledged write is in object storage before the ACK returns to the
client — is the central promise of clustered mode. A handoff that
loses commits because a write wasn't actually flushed is the
existence-justifying bug class for these tests.

## Audit findings addressed

- **F2 test bodies (Critical, journey-gap, all four taxonomy layers)** —
  Object-storage-sync has zero coverage. (The MinIO fixture itself lives
  in `epic-e2e-cnd-coverage-cluster-fixture`; this feature owns the test
  bodies.)
- **F10 (Medium, missing-taxonomy-layer fuzz)** — Object-storage URL-
  scheme parser has no fuzz coverage. New attack surface added by CND:
  `s3://`, `gs://`, `azblob://`, `s3-compatible://` schemes.
- **F12 (Medium, missing-taxonomy-layer fuzz)** — Pack manifest format
  has no fuzz coverage. Pack manifests are the linearizability anchor
  for object storage; corruption at parse time should fail loudly.

## Scope

### Tests to add

1. **`tests/e2e/golden/object_storage_rpo0_test.go`**
   - Push a commit to a session. Before the push returns 2xx, assert the
     pack object exists in MinIO (direct bucket inspection). RPO=0
     invariant: ACK implies durable.
   - Subtest variations: small commit, multi-pack push, refs-only update
     (`packed-refs` write path), tag creation.
   - **Invariant statement**: "After a successful push, every produced
     object (objects/, packs, refs) is queryable in the MinIO bucket
     via direct S3 API."

2. **`tests/e2e/golden/object_storage_pack_manifest_test.go`**
   - After a push, the pack manifest in MinIO is well-formed and
     references all produced packs (no dangling references, no missing
     entries). Read manifest directly from bucket; parse with a
     deliberately-strict parser in the test (test code can read manifest
     format from docs); assert against actual pack object names in bucket.
   - **Invariant**: "Pack manifest is a true description of the bucket
     state at any read-after-write moment."

3. **`tests/e2e/failure/object_storage_unreachable_at_startup_test.go`**
   - **Subtest `clustered_mode_fails_fast`** — clustered portal configured
     with an unreachable `JAMSESH_OBJECT_STORE_URL` (point at a non-
     existent host). Portal exits non-zero with `obj_store.unreachable`
     log line.
   - **Subtest `single_instance_unaffected`** — same portal binary in
     single-instance mode (`JAMSESH_DEPLOY_MODE=single`) with the same
     bad URL ignored; portal serves normally. This is the
     "clustered-mode knob defaults to off" invariant from the parent
     CND epic.

4. **`tests/e2e/failure/object_storage_write_rejected_test.go`**
   - Misconfigure MinIO IAM to deny writes (or use a bucket the portal
     wasn't granted PutObject on). Attempt a push. Portal returns a
     server-error response with a documented error code; push does not
     "succeed silently while losing the object". RPO=0 means an
     un-persistable write must surface as failure.

5. **`tests/e2e/chaos/object_storage_partition_test.go`**
   - MinIO + Toxiproxy in front. Inject latency (e.g., 5s delay) on
     portal→MinIO; assert writes still succeed within retry budget.
   - Then inject `reset_peer` for a bounded window; assert writes that
     are in-flight retry and eventually succeed once the partition heals,
     and that no acknowledged write is lost (RPO=0 holds across the
     chaos window).
   - Then inject permanent disconnect; assert writes start failing with
     the documented error code (no silent success).

6. **`tests/e2e/fuzz/object_storage_dsn_test.go`** (F10)
   - Property-based fuzz on `JAMSESH_OBJECT_STORE_URL`: random values
     drive portal startup. Property — any input either parses to a
     valid config and the portal boots, or fails fast with a typed error.
     Never panic, never SEGV, never boot-then-crash-on-first-write.
   - Seed corpus: schemes with malformed authority, query strings,
     fragments, path traversal (`s3://bucket/../etc/passwd`), embedded
     newlines, unicode, percent-encoding edge cases, double-scheme
     (`s3://s3://`), wrong-scheme (`https://`), empty.

7. **`tests/e2e/fuzz/pack_manifest_test.go`** (F12)
   - Write malformed pack manifests directly into the MinIO bucket
     (out-of-band, via direct S3 client). Then trigger a hydrate-like
     read on the portal (e.g., a session-list query that requires
     consulting the manifest). Property — read either succeeds with a
     verified manifest or refuses with a typed error. No silent
     truncation, no panic, no boot with corrupt cache.
   - Seed corpus: truncated JSON, oversize, wrong schema, valid JSON
     with references to non-existent pack objects, valid JSON with
     ordering inversions.

### Helpers

- A MinIO bucket inspector — list objects, read content, assert
  presence. Lives in `tests/e2e/fixtures/minio/inspect.go`.
- An IAM-restricted MinIO variant — helper to construct a MinIO instance
  with a deny-writes policy applied. Used for F4 (write rejected).

## Mock-boundary plan

| External dep              | Service-level mock              | Notes |
|---------------------------|---------------------------------|-------|
| S3 / S3-compatible        | MinIO (from cluster-fixture)    | Off-the-shelf service mock |
| Network partition         | Toxiproxy in front of MinIO     | Existing chaos pattern |
| Pack-manifest format      | Real portal parses real bucket  | Test code reads manifest via S3 SDK directly — testing the portal's behavior, not a parser stub |
| Provider-specific paths   | Out of scope (see non-goals)    | GCS/Azure SDK glue covered in unit tests |

No in-process mocks. The fuzz harnesses drive real HTTP and real S3 paths
through real backends; the only thing "fuzzed" is the input data.

## Open questions for design

- **MinIO IAM constraint shape.** MinIO supports policy JSON; design pass
  picks the simplest policy that denies PutObject while allowing connection
  + bucket-exists checks. Tested by hand against a local MinIO before
  encoding into the fixture.
- **Pack manifest format spec.** Where does the canonical manifest format
  live? `docs/ARCHITECTURE.md` Horizontal Scaling subsection? A research
  doc? Design pass locates it without reading implementation; if no doc
  spec exists, file a `documentation` finding (foundation-doc drift) and
  derive the format from probe calls.
- **Toxiproxy + MinIO ordering.** Toxiproxy fronts portal→MinIO; means
  portal's MinIO endpoint URL points at Toxiproxy's listen address.
  Bucket-create has to either happen via Toxiproxy too (so the bucket
  setup works under chaos conditions) or via a direct-to-MinIO admin
  channel. Design pass decides; lean toward direct-to-MinIO for setup,
  through-Toxiproxy for the workload.
- **Hydrate-trigger for F12 (pack manifest fuzz).** What's the cleanest
  way to force a portal to re-read the manifest? Cold start of a pod?
  Eviction + re-acquire? Or a dedicated test endpoint? Resolve in design.

## Acceptance criteria

- [ ] `object_storage_rpo0_test.go` green; pack object queryable in bucket
      before push returns ACK
- [ ] `object_storage_pack_manifest_test.go` green; manifest matches
      bucket reality
- [ ] `object_storage_unreachable_at_startup_test.go` green; clustered
      fails fast, single-instance unaffected
- [ ] `object_storage_write_rejected_test.go` green; IAM-denied write
      surfaces as documented error (no silent loss)
- [ ] `object_storage_partition_test.go` green; latency + transient
      reset + permanent disconnect all produce the right behavior
- [ ] `object_storage_dsn_test.go` green; full fuzz corpus; no panic;
      every input either boots cleanly or fail-fast typed error
- [ ] `pack_manifest_test.go` green; corrupted manifests refuse cleanly
      (no silent truncation)
- [ ] MinIO bucket-inspect + IAM-denied helpers landed in the MinIO
      fixture
- [ ] No new in-process mocks introduced
- [ ] Any production bugs surfaced (e.g., RPO=0 violation under chaos,
      or silent acceptance of corrupted manifest) parked per
      test-integrity rules

## Test integrity (from parent epic)

The RPO=0 invariant is exactly the property tests can lie about most
easily — "the push returned 2xx, therefore RPO=0 holds" without actually
checking the bucket is a tautology. **Every RPO test must directly
inspect the bucket via S3 API, not just assert on portal HTTP responses.**

If the chaos test surfaces a real RPO violation (an ACKed write that
never landed in the bucket), park the bug via `/agile-workflow:park`,
land the test with a clear failure message naming the inv ariant
violation; do NOT loosen the assertion to "eventually consistent" unless
docs explicitly say so.

## Non-goals

- **Provider-specific e2e coverage for GCS, Azure.** S3-compat via MinIO
  covers the invariants; provider-SDK glue (workload identity, generation
  match) is unit-test territory.
- **Multi-bucket / multi-region.** Out of CND scope.
- **Performance / throughput characterization.** Perf-design territory.
- **Backup / restore from bucket snapshots.** Separate concern.

## Next

`/agile-workflow:e2e-test-design epic-e2e-cnd-coverage-object-storage-sync`
