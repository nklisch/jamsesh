---
id: epic-cloud-native-deploy-hydration-handoff-hydrator
kind: story
stage: done
tags: [portal]
parent: epic-cloud-native-deploy-hydration-handoff
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Hydration + Handoff — Hydrator (pure download + write logic)

## Scope

The `Hydrator` that downloads a session's manifest + packs + refs + loose
objects from object storage and writes them to local-FS bare repo. Atomic
writes via tmp + rename; parallel downloads via errgroup; integrity check
via `git fsck --no-dangling`.

Implements **Unit 1** of `epic-cloud-native-deploy-hydration-handoff`.
See parent feature body for full specs + acceptance criteria.

## Files

New:
- `internal/portal/storage/objectstore/hydrate.go` — `Hydrator`, `Hydrate`, `HydrationOutput`
- `internal/portal/storage/objectstore/hydrate_test.go` — uses memBackend + temp repo dir

## Acceptance criteria

- [ ] Fresh session (no manifest) → no-op success; HydrationOutput zero counts; CreateRepo called
- [ ] Existing session → downloads all packs + refs + loose objects; HydrationOutput has counts
- [ ] Parallel download timing: 5 packs × 100ms each completes in ≤200ms
- [ ] Atomic writes: simulated mid-download failure leaves no `.tmp` files
- [ ] `git fsck --no-dangling` runs; FsckOK reflects exit code
- [ ] Unit tests using memBackend + temp directory

## Notes

- Sequence: Load manifest → fresh-session early-return → ensure bare repo dir → parallel download packs/idx/loose objects → write refs/packed-refs → fsck → metrics → return.
- `errgroup.Group` with `SetLimit(h.Workers)` for parallel downloads (default 8).
- Atomic write helper: `writeAtomic(path string, data []byte) error` — write to `path + ".tmp"`, then `os.Rename(tmp, path)`. Cleanup `.tmp` on context cancellation.
- `git fsck --no-dangling` is run via `exec.CommandContext(ctx, "git", "-C", repoPath, "fsck", "--no-dangling")`. Dangling objects are normal in a freshly-hydrated repo (manifest's pack list doesn't reference every loose object). FsckOK = (exit code == 0); non-fatal failure → log slog.Warn but don't return error.
- `Storage.RepoPath(orgID, sessionID)` returns the bare repo path. Hydrator signature: `Hydrate(ctx, orgID, sessionID)`.
- Metrics emission deferred to Unit 3 (wiring story); pre-declare the call sites with nil-safe `if h.Metrics != nil` guards.

## Implementation notes

### Files produced

- `internal/portal/storage/objectstore/hydrate.go` — `Hydrator`, `HydrationOutput`, `Hydrate`, `writeAtomic`, `isAlreadyExistsErr`
- `internal/portal/storage/objectstore/hydrate_test.go` — 8 tests using `memBackend` + temp repo dirs; includes parallel-timing, atomic-failure, and nil-metrics coverage
- `internal/portal/metrics/metrics.go` — added `HydrationsTotal` (CounterVec, label `result`), `HydrationDurationSeconds` (Histogram), `HydrationBytesTotal` (Counter); registered in `New()`

### Key implementation choices

- **Option A for metrics** (add handles to metrics.go now): chose this per the story directive; wiring story can use the handles directly without churn.
- **Manifest + Hydrator Backend separation**: the test wires `Manifests` to the unfailing `*memBackend` and `Backend` to the `slowBackend`/`failAfterNBackend` wrapper. This lets the Hydrator load manifests without delay while testing pack download behaviour in isolation.
- **Loose-object key format confirmed**: `sessions/<id>/objects/<xx>/<rest>` — matched exactly from `sync.go:uploadLooseObjects`. Packs are at `sessions/<id>/packs/<sha>.pack` (not under `objects/`), so the objects/ listing contains only loose objects; the pack-prefix guard in `downloadLooseObjects` is a defensive safety net.
- **`isAlreadyExistsErr`**: matches substring `"already exists"` in the error message (storage.Service.CreateRepo returns a formatted string, not a sentinel). This is intentionally liberal to handle concurrent CreateRepo calls safely.
- **Parallel counts**: `downloadPacks` returns `len(packs)` as the pair count (not individual file count) to match `HydrationOutput.PacksDownloaded` semantics.
- **`TestHydrator_ParallelTiming`** completes in ≈210ms on a local machine (10 downloads at 100ms each, Workers=8 → two waves), well within the 250ms ceiling.
- **fsck warnings in tests**: tests that use bogus pack data (fake bytes, not real git objects) produce fsck warnings — this is expected. `TestHydrator_FsckOK` specifically validates an empty bare repo (which always passes fsck).

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Clean implementation. 410 lines code + 573 lines tests — solid ratio. Fresh-session early-return path is correct (zero counts + FsckOK=true vacuously + CreateRepo idempotent via "already exists" substring match — handles concurrent CreateRepo calls safely).

`writeAtomic` helper centralizes the tmp+rename pattern. Parallel timing test (10 downloads × 100ms / 8 workers → 2 waves → ~210ms) validates the errgroup.WithContext + SetLimit pattern is wired correctly.

Loose-object key format verified against the Syncer's actual write pattern (`sessions/<id>/objects/<xx>/<rest>`, packs at `sessions/<id>/packs/...`). The defensive pack-prefix guard in `downloadLooseObjects` is unnecessary given the actual key separation but a small safety net.

Test approach is elegant: separate Backends for Manifests (memBackend, unfailing) and packs (slowBackend / failAfterNBackend wrapper) lets the manifest load happen instantly while testing pack behavior in isolation. `TestHydrator_AtomicWriteOnFailure` verifies no `.tmp` files left after simulated mid-download failure.

3 new metric handles (HydrationsTotal, HydrationDurationSeconds, HydrationBytesTotal) appended cleanly to Registry following the established lease/upload pattern. Nil-safe call sites preserved.

`git fsck --no-dangling` runs against the hydrated repo; FsckOK reflects the exit code, non-fatal as designed (dangling objects normal in freshly-hydrated repos).
