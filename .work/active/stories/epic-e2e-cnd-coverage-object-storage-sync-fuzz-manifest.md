---
id: epic-e2e-cnd-coverage-object-storage-sync-fuzz-manifest
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

# Object Storage — Fuzz: Pack Manifest Reader (F12)

Implements `tests/e2e/fuzz/pack_manifest_test.go` and
`tests/e2e/fuzz/testdata/pack-manifest-corpus.json`.

Addresses audit finding F12 (Medium, missing-taxonomy-layer fuzz): the pack
manifest format (`objectstore.Manifest`) has no fuzz coverage. Pack manifests
are the linearizability anchor for object storage; corruption at parse time
should fail loudly.

## Invariant (property)

Any bytes at `sessions/<sessionID>/manifest.json` in MinIO either:
- Parse as a valid `objectstore.Manifest` (version=1, session_id non-empty,
  all PackEntry keys exist in bucket) and the portal proceeds normally, OR
- Cause the portal to fail fast with a typed error on first hydration.

The portal NEVER:
- Panics or nil-derefs on manifest parse
- Silently truncates a corrupt manifest and proceeds as if it were valid
- Accepts a manifest with dangling pack references and silently produces an
  inconsistent state

## Scope

`TestPackManifestFuzz`:

- Skip under `-short`.
- For each seed in `testdata/pack-manifest-corpus.json`:
  1. Start MinIO. Pre-seed the bucket: `mn.PutObject(ctx,
     "sessions/"+sessionID+"/manifest.json", seedBytes)`.
  2. Start a **1-pod cluster** with that bucket (cold-start triggers hydration
     which reads the manifest).
  3. Attempt a git push to the session.
  4. Property check:
     - If push returns non-2xx: correct — the portal rejected the corrupt manifest.
       Assert: no 5xx from a nil-pointer (portal panics are production bugs).
     - If push returns 2xx: only acceptable if the seed was a valid manifest.
       For invalid seeds, 2xx means silent truncation — a production bug.
  5. Assert: portal did not log "panic" or unhandled nil-pointer.

## Seed corpus: `testdata/pack-manifest-corpus.json`

Minimum 10 entries:

```json
[
  {"description": "empty bytes", "manifest": ""},
  {"description": "null JSON", "manifest": "null"},
  {"description": "truncated JSON", "manifest": "{\"version\":1,"},
  {"description": "wrong version type", "manifest": "{\"version\":\"one\",\"session_id\":\"x\",\"packs\":[],\"refs\":{}}"},
  {"description": "unknown version 99", "manifest": "{\"version\":99,\"session_id\":\"x\",\"packs\":[],\"refs\":{}}"},
  {"description": "dangling pack reference", "manifest": "{\"version\":1,\"session_id\":\"PLACEHOLDER\",\"packs\":[{\"pack_key\":\"sessions/PLACEHOLDER/packs/nonexistent.pack\",\"idx_key\":\"sessions/PLACEHOLDER/packs/nonexistent.idx\",\"sha\":\"abc\"}],\"refs\":{}}"},
  {"description": "oversize 5MB", "manifest": "<5MB JSON blob>"},
  {"description": "duplicate keys", "manifest": "{\"version\":1,\"version\":2,\"session_id\":\"x\",\"packs\":[],\"refs\":{}}"},
  {"description": "empty session_id", "manifest": "{\"version\":1,\"session_id\":\"\",\"packs\":[],\"refs\":{}}"},
  {"description": "valid manifest no packs", "manifest": "{\"version\":1,\"session_id\":\"PLACEHOLDER\",\"packs\":[],\"refs\":{}}"}
]
```

Seeds with `PLACEHOLDER` get their session ID substituted at test time.
The last entry ("valid manifest no packs") should produce a 2xx from a push
(no packs expected yet; objects will be new loose objects). This is the
control seed that verifies the harness itself works.

**Test integrity rules (mandatory for implementer)**:
- Silent truncation (corrupt manifest accepted, push returns 2xx, but state
  is inconsistent) is a production bug. Park it via `/agile-workflow:park`,
  skip that seed's sub-test with the backlog ID. Do NOT change the assertion
  to accept 2xx on clearly invalid seeds.
- A panic (nil-deref) in portal logs is a production bug. Park it.
- The "control seed" (valid manifest) MUST pass. If it fails, the harness
  setup is broken — fix the setup before running any other seeds.

## Acceptance Criteria

- [ ] `TestPackManifestFuzz` compiles and runs (skip under `-short`)
- [ ] `testdata/pack-manifest-corpus.json` has ≥10 entries covering all
      categories above
- [ ] No seed causes a panic/nil-deref in portal logs
- [ ] No invalid seed causes silent truncation (corrupt manifest + 2xx push)
- [ ] The control seed (valid manifest, no packs) produces a 2xx push
- [ ] Any production bugs found are parked with backlog IDs, not silenced
- [ ] No in-process mocks introduced

## Notes

- The depends_on on `golden-rpo0` is for the session-creation and git-push
  helpers (`createSessionAndToken`, `gitPush`) that this test reuses.
- Each seed uses a fresh 1-pod cluster (cold-start hydration) — this test is
  intentionally slow. Skip with `-short`.
- The session ID in seeds marked `PLACEHOLDER` must be substituted with the
  real session ID from the test run. Pre-seed the bucket before starting the
  cluster so the pod's cold-start reads the seeded manifest.

## Implementation Notes (2026-05-17)

- Implemented `tests/e2e/fuzz/pack_manifest_test.go` and
  `tests/e2e/fuzz/testdata/pack-manifest-corpus.json`.
- Corpus has 15 entries covering all categories from the story design:
  empty, null, truncated, wrong types, unknown version, dangling references,
  oversize, duplicate keys, missing fields, session ID mismatch, and the
  control seed (valid manifest, no packs).
- PLACEHOLDER substitution is applied at test time; each seed operates in a
  fresh session namespace so seeds cannot pollute each other.
- Cold-start hydration trigger: each seed starts a 1-pod cluster against
  the pre-seeded bucket. Both clone and push paths are checked for panics.
- Control seed runs first (sequential) to verify harness correctness before
  exercising invalid seeds in parallel.
- Random phase: 5 iterations by default (MANIFEST_FUZZ_COUNT to override),
  each generating a random malformed manifest across 12 shape categories.
- `go build ./fuzz/... && go vet ./fuzz/...` pass cleanly.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Design said to `t.Skip` with a backlog ID when silent truncation is detected;
  implementation uses `t.Errorf` instead, which is stricter (keeps the suite red
  until the bug is fixed). The inline comment "Do NOT change the assertion to accept
  2xx on clearly invalid seeds" documents the intent. Defensible deviation — more
  honest than skip-and-forget.
- The "oversize 5MB" seed from the design is in the random generator rather than
  the static corpus. The corpus covers the other 14+ required categories; the oversize
  shape is exercised in the random phase.

**Notes**: PLACEHOLDER substitution is applied correctly at test time. Control seed
runs first (sequential) to validate the harness before exercising invalid seeds.
Panic detection is applied at both clone-time and push-time. Bootstrap + hot-cluster
separation ensures cold-start hydration reads the pre-seeded manifest. No in-process
mocks. Corpus has 15 entries (≥10 required). Random phase exercises 12 shape
categories. Silent truncation path triggers `t.Errorf` with a clear directive
to park — no silent acceptance.
