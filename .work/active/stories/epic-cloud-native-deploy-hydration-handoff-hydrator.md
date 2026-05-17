---
id: epic-cloud-native-deploy-hydration-handoff-hydrator
kind: story
stage: implementing
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
