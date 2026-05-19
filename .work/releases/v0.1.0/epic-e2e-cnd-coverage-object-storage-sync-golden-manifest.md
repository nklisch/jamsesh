---
id: epic-e2e-cnd-coverage-object-storage-sync-golden-manifest
kind: story
stage: done
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-object-storage-sync
depends_on: [epic-e2e-cnd-coverage-cluster-fixture, epic-e2e-cnd-coverage-object-storage-sync-golden-rpo0]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object Storage — Golden Pack Manifest Integrity

Implements `tests/e2e/golden/object_storage_pack_manifest_test.go`.

## Invariant

The pack manifest at `sessions/<sessionID>/manifest.json` in the MinIO bucket
is a true description of the bucket state after every push. Every `PackEntry`
in `manifest.Packs` has both its `.pack` and `.idx` objects present in the
bucket. No dangling references, no missing entries.

## Scope

`TestObjectStoragePackManifest` with at least two subtests:

- **`manifest_matches_bucket_after_push`** — after a push, read
  `sessions/<id>/manifest.json` via `mn.GetObject`, unmarshal into
  `objectstore.Manifest`, then for each `PackEntry` assert that
  `mn.GetObject(entry.PackKey)` and `mn.GetObject(entry.IdxKey)` both succeed.
- **`manifest_session_id_and_version`** — assert `manifest.SessionID == sessionID`
  and `manifest.Version == 1`.

**Test integrity rules (mandatory for implementer)**:
- Read the manifest bytes directly from MinIO via `mn.GetObject`; do NOT
  call any portal API to ask "what's in the manifest?". The portal API would
  be tautological — we are testing the portal's own behavior.
- If a `PackEntry` references a key that doesn't exist in the bucket, this is
  a production bug (dangling manifest reference). Park it via
  `/agile-workflow:park`, mark the subtest as `t.Skip` with the backlog ID
  and reason. Do not change the assertion.
- Fix bad fixtures in-session. Never game an assertion.

## Acceptance Criteria

- [ ] `TestObjectStoragePackManifest` compiles and runs against the cluster-fixture stack
- [ ] Manifest is read directly from MinIO bucket, not from any portal API
- [ ] Every PackEntry's `.pack` and `.idx` keys exist in the bucket
- [ ] `manifest.SessionID` and `manifest.Version` are asserted
- [ ] Any production bugs (dangling manifest references) are parked, not silenced
- [ ] No in-process mocks introduced

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Manifest is read exclusively via `mn.GetObject` — no portal API calls,
fully non-tautological. Bidirectional consistency check is present: manifest→bucket
(every PackEntry's keys exist) AND bucket→manifest (every bucket pack object is
referenced in the manifest). The inlined `manifestJSON` mirror struct matches the
production `objectstore.Manifest` JSON tags exactly; the two extra production fields
(`fencing_token`, `updated_at`) that are absent from the mirror do not affect
correctness since they aren't asserted on. Module boundary constraint (no cross-
module imports) is correctly handled with the inlined struct approach. Infrastructure
setup matches rpo0 test. Helpers reused correctly from package-scoped definitions.

## Notes

- Import `jamsesh/internal/portal/storage/objectstore` to reuse the
  `Manifest` and `PackEntry` structs for unmarshaling — the test reads the
  real production struct.
- The depends_on on `golden-rpo0` is for shared push helpers (e.g.
  `createSessionAndToken`, `gitPush`), not a functional ordering requirement.

## Implementation Notes (2026-05-17)

- `tests/e2e` is a separate Go module (`jamsesh/tests/e2e`) with no go.work
  workspace and no replace directive pointing at the main module. Direct import
  of `jamsesh/internal/portal/storage/objectstore` is not possible across the
  module boundary. The production struct's JSON tags are the stable contract;
  `manifestJSON` and `packEntryJSON` are inlined mirrors with identical json
  tags. Build + vet guard against drift.
- `manifestKey()` local helper replicates `objectstore.ManifestKey()` (same
  formula: `"sessions/" + sessionID + "/manifest.json"`).
- Infrastructure stack: 2-pod `portalcluster` + MinIO + Postgres + MailHog
  (SMTP for magic-link auth), mirroring the rpo0 test exactly.
- Shared helpers from `object_storage_rpo0_test.go` used: `randEmail`,
  `rpo0GetMe`, `rpo0CreateSession` (all in package `golden_test`).
- Subtest `manifest_matches_bucket_after_push`: reads manifest, checks every
  PackEntry's .pack and .idx exist in bucket, and checks every bucket pack
  object appears in the manifest (bidirectional consistency).
- Subtest `manifest_session_id_and_version`: asserts `SessionID == sessionID`
  and `Version == 1`.
- No production bugs encountered during implementation; no subtests skipped.
