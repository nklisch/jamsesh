---
id: e2e-cloud-native-multipod-suite-red-fuzz
kind: feature
stage: done
tags: [portal, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: []
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
---

# Fuzz suite stabilization

## Brief
The fuzz suite is red and uncharacterized. Characterize each harness's failure,
then resolve per the project test-integrity rules: genuine product
input-handling bugs are fixed; stale seeds / harness drift / outdated assertions
are repaired in-session. Goal is a green `tests/e2e/fuzz/...` suite.

This feature is independent of the subsystem fixes and runs in parallel. Per the
parent epic's design decisions this is never-green stabilization — characterize
and root-cause forward from the current red state, no bisect.

## Epic context
- Parent epic: `e2e-cloud-native-multipod-suite-red`
- Position in epic: independent capability — parallel with all subsystem fixes;
  nothing depends on it.

## Foundation references
- `docs/ARCHITECTURE.md` — the surfaces each harness fuzzes
- Harnesses: `tests/e2e/fuzz/fencing_token_test.go`,
  `mcp_tool_input_test.go`, `object_storage_dsn_test.go`,
  `pack_manifest_test.go`, `playground_nickname_test.go`

## Run target (CORRECTED)
The red suite is the ordinary go-test suite under `tests/e2e/fuzz/`, run as:

```
cd tests/e2e && go test ./fuzz/... -count=1
```

NOT `make test-fuzz` — that target fuzzes a *different* package
(`internal/portal/prereceive`, via `-fuzz=...`) and is unrelated to the red e2e
workflow. These harnesses are seed-corpus / property-assertion style tests that
drive real Docker testcontainers (portalcluster / minio / postgres / mailhog)
against the `jamsesh/portal:e2e` image. Under autopilot, wrap every run in the
shared e2e lock and point Go's tmp at the cache disk (/tmp is tmpfs):

```
flock /tmp/jamsesh-autopilot-e2e.lock bash -c \
  'cd tests/e2e && GOTMPDIR=$HOME/.cache/gotmp TMPDIR=$HOME/.cache/gotmp \
   go test ./fuzz/... -count=1 -timeout 1500s'
```

A generous `-timeout` is needed: the container-heavy harnesses (manifest,
fencing) serialise their cold-starts (see "Infra fix" below) and run long.

## Per-harness characterization (from the red baseline)

Baseline `go test ./fuzz/... -count=1 -v`:

| Harness | Top-level | Root cause |
|---|---|---|
| `TestObjectStorageDSNFuzz` | PASS | — |
| `TestPlaygroundNicknameFuzz` / `FuzzPlaygroundNickname` | PASS | — |
| `TestMCPToolInputFuzz` | PASS | — |
| `TestFencingTokenRejectionIsExplicit` | PASS | — |
| `TestFencingTokenFuzz` | FAIL | INFRA only — portal container startup stalls |
| `TestPackManifestFuzz` | FAIL | TWO causes — (A) startup stalls; (B) real product bug + (C) one drifted assertion |

Two distinct root causes, plus one assertion-drift:

### (A) INFRA / test-debt — portal container startup stalls
Symptom: `cluster.go:132: ... wait until ready: context deadline exceeded`
followed by `portalcluster.Start: pod 0 is nil after startup`. The portal logs
`"portal starting"` (binary boots) but `/healthz` does not return 200 within the
readiness deadline.

Evidence it is infra, not product:
- Healthy portal boots are bimodal: p99 ≈ 1s, max 1s; stalls jump straight to
  the full deadline. Not "slow startup" — transient hangs.
- The `pack_manifest` parallel seeds all started and timed out at the *same*
  wall-clock second (Go's default `-parallel` = GOMAXPROCS = 16), i.e. a fan-out
  of full portal+pg+minio stacks saturating the Docker host.
- Fully serialised single boots come up cleanly. Remaining stalls at
  concurrency=1 are isolated single-container events (~4 per ~80 boots) — a
  classic transient testcontainers/Docker-host flake.

This is test/infra flakiness (test debt). Fixed in-session via three mitigations
in the shared portal fixture + a fuzz-package limiter (see "Fixes applied").

### (B) REAL PRODUCT BUG — silent acceptance of a corrupt session manifest
Symptom (pack_manifest, fast-failing seeds `null JSON`, `unknown version 99`,
`duplicate version key`, plus random iterations):
`BUG DETECTED (silent truncation) — portal accepted a corrupt manifest and
returned 2xx from git push.`

Root cause: `internal/portal/storage/objectstore/manifest.go` —
`ManifestStore.Load` decoded the manifest with a lenient `json.Unmarshal` and
returned whatever fell out, with **no validation**. Go's `encoding/json` happily
decodes `null` / `{}` / `{"version":99,...}` / a tampered `session_id` into a
degenerate `Manifest`. Hydration (`hydrate.go`) then treats the corrupt object
as an empty/fresh session, hydrates an empty repo, and the subsequent push
returns 2xx — silently dropping the session's real pack/ref history (a
durability / correctness violation). This contradicts the `Manifest` type's own
documented contract: *"Unknown versions should be treated as unreadable by
future code — return an error rather than silently mis-parsing."*

This is a genuine, small, clearly-correct fix (aligns the code with its
documented contract; two callers — `Hydrate` and `Sync` pre-flight — already
propagate `Load` errors), so it was fixed in-session rather than filed. No
backlog item was needed.

### (C) Test-debt — one drifted pack_manifest assertion (`packs:null`)
Seed `seed_08 "packs is null instead of array"` was classified must-reject, but
`{"version":1,"session_id":"<sid>","packs":null,"refs":{}}` is in fact a VALID
manifest: Go marshals a nil `[]PackEntry` slice as JSON `null`, so a manifest the
portal itself writes with no packs serialises with `"packs":null`. A 2xx push is
the correct outcome. The harness's blanket "all non-control seeds must reject"
mis-flagged it. Fixed by reclassifying this seed as a valid-manifest variant
(asserted to succeed like the control).

## Test-debt vs product-bug split
- **Product bug (fixed in product code):** (B) — manifest `Load` accepted
  corrupt manifests. Fixed in `internal/portal/storage/objectstore/manifest.go`.
- **Test debt (fixed in-session):** (A) container-startup flakiness in the shared
  e2e portal fixture; (C) the `packs:null` drifted assertion.

## Fixes applied

### Product (rebuilt into `jamsesh/portal:e2e`)
- `internal/portal/storage/objectstore/manifest.go`
  - New sentinel `ErrCorruptManifest` and `manifestSchemaVersion = 1`.
  - `Load` now validates a manifest that exists in storage: it must decode to
    `Version == 1` and `SessionID == <requested session>`, else it returns
    `ErrCorruptManifest` (wrapped). Type-mismatch bodies (e.g. `packs:"string"`)
    already errored at unmarshal. Missing manifest (fresh session) is unchanged.
  - Updated the `Load` doc comment.
- `internal/portal/storage/objectstore/manifest_test.go`
  - New `TestManifestStore_Load_Corrupt` covering `null`, `{}`, version 99,
    version 0, duplicate-version, empty session_id, and session_id mismatch —
    each must surface `ErrCorruptManifest`.

Net effect through the request path: a corrupt manifest makes `Hydrate` fail
fast (`lifecycle.acquireNew` releases the lease and returns the error), so the
git clone/push is rejected (non-2xx) instead of silently truncating. Fencing's
valid-int64-token cases are unaffected (those manifests are version-1 /
matching-session), and fencing's non-integer-token cases were already expected
to be rejected — so the product fix is fully compatible with the fencing harness.

### Infra / test-debt (no image rebuild — test-only)
- `tests/e2e/fixtures/portal/portal.go`
  - Container start is now **retried** (3 attempts, 60s readiness ceiling each):
    a transient single-boot stall fails the attempt and a fresh container is
    started; healthy first attempts are unaffected. Failed half-started
    containers are terminated between attempts.
- `tests/e2e/fuzz/limiter_test.go` (new)
  - Process-wide container-startup semaphore (default concurrency **1**,
    override via `FUZZ_STARTUP_CONCURRENCY`). The slot is released via
    `t.Cleanup` registered *before* the cluster fixtures, so (LIFO) it is held
    until this seed's containers are fully torn down — serialising the expensive
    boot+teardown I/O across the whole package and removing the fan-out that
    saturated the Docker host.
- `tests/e2e/fuzz/pack_manifest_test.go`, `tests/e2e/fuzz/fencing_token_test.go`
  - Each seed runner acquires a startup slot before booting clusters.
  - pack_manifest: reclassified the `packs:null` seed (C) as a valid-manifest
    variant that must push successfully.

## Filed to backlog
None. The single product bug found (B) was small and clearly correct, so it was
fixed in-session per the test-integrity rules rather than parked.

## Out-of-scope observations
- The container-startup flakiness lives in the *shared* e2e portal fixture and
  affects every cnd-coverage suite, not just fuzz. The mitigations here (retry +
  generous per-attempt timeout) benefit all of them; the concurrency limiter is
  fuzz-package-local. The Docker daemon is a shared singleton across parallel
  autopilot worktrees, so a concurrently-running sibling e2e suite can still
  induce host contention regardless of this suite's own concurrency cap.

## Verification
Commands (under the shared lock, GOTMPDIR/TMPDIR on the cache disk):

```
# product fix unit coverage
go test ./internal/portal/storage/objectstore/... -count=1     # PASS

# full fuzz suite
flock /tmp/jamsesh-autopilot-e2e.lock bash -c \
  'cd tests/e2e && GOTMPDIR=$HOME/.cache/gotmp TMPDIR=$HOME/.cache/gotmp \
   go test ./fuzz/... -count=1 -timeout 1500s'
```

Result: see commit / run log. Across all post-fix runs the product-bug signal
(`silent truncation`) is fully eliminated (0 occurrences); the remaining risk is
the shared-host container-startup transient, mitigated by start-retry +
serialised cold-starts.
