---
id: epic-cloud-native-deploy-object-storage-sync-manifest
kind: story
stage: implementing
tags: [portal]
parent: epic-cloud-native-deploy-object-storage-sync
depends_on: [epic-cloud-native-deploy-object-storage-sync-backend]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object-Storage Sync — Pack manifest + state model

## Scope

The per-session linearizable state object stored at
`sessions/<id>/manifest.json` listing current pack files, refs,
packed-refs content, and high-water fencing token. Read-side index that
hydration uses; conditional-write target that makes session state
linearizable on top of an eventually-consistent backend.

Implements **Unit 2** of `epic-cloud-native-deploy-object-storage-sync`.

## Files

New:
- `internal/portal/storage/objectstore/manifest.go` — `Manifest` struct +
  `ManifestStore{Load, Save}` + `ErrFenced` sentinel
- `internal/portal/storage/objectstore/manifest_test.go`

## Acceptance criteria

- [ ] `Load` on missing manifest returns zero-value Manifest, empty ETag, nil
- [ ] `Load` on existing manifest returns it + the current ETag
- [ ] `Save` with `ifMatch=""` succeeds when manifest doesn't exist;
  returns `ErrPrecondition` when it does
- [ ] `Save` with matching `ifMatch` succeeds; returns new ETag
- [ ] `Save` with stale `ifMatch` returns `ErrPrecondition`
- [ ] `Save` with fencing token < on-disk token returns `ErrFenced`
  (distinct from `ErrPrecondition` — operational meaning differs)
- [ ] `Save` with fencing token ≥ on-disk + matching `ifMatch` succeeds
- [ ] JSON round-trip is lossless across all fields

## Notes

- `ErrFenced` is the "your lease is stale, abort and 503" signal.
  `ErrPrecondition` is the "concurrent writer won, retry" signal.
  Callers handle them differently.
- Caller pattern is read-modify-write: `Load → mutate → Save(ifMatch=oldEtag)`.
- The fencing-token validation happens IN Save (after reading the
  on-disk manifest's token). This catches stale-lease-holder writes
  that a pure ETag check wouldn't catch (the stale holder could have
  read an old manifest, mutated correctly per its view, and written
  with the right ETag — but with a stale token).
