---
id: epic-cloud-native-deploy-object-storage-sync-manifest
kind: story
stage: done
tags: [portal]
parent: epic-cloud-native-deploy-object-storage-sync
depends_on: [epic-cloud-native-deploy-object-storage-sync-backend]
release_binding: v0.1.0
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

## Implementation notes

### Design decisions

**Create-only guard in ManifestStore.Save**: The Backend.Put contract treats
`ifMatch=""` as an unconditional overwrite (create-or-overwrite). However,
the manifest layer's contract for `Save(ifMatch="")` is create-only: if a
manifest already exists, it must return ErrPrecondition. This guard lives in
`ManifestStore.Save` itself, using the ETag returned by the fencing pre-flight
Load to detect an existing manifest. If `onDiskEtag != ""` and `ifMatch == ""`,
Save returns ErrPrecondition before touching the Backend.

**Fencing pre-flight reuses the Load result**: The same Load call used for the
fencing token comparison also supplies the `onDiskEtag` for the create-only
guard. This avoids a second round-trip to the Backend and keeps the logic in
a single place.

**Save defaults Version and UpdatedAt**: Callers do not need to set these.
`Version` defaults to 1 if zero; `UpdatedAt` is always overwritten with
`time.Now().UTC()` so on-disk values are always authoritative.

### Test approach

Pure unit tests using an in-memory `memBackend` (defined in test file).
No S3 / MinIO required. The memBackend correctly implements all five Backend
methods with ETag conditional-write semantics, byte-for-byte PutIdempotent
comparison, and mutex-guarded map storage.

Test coverage includes all acceptance criteria plus two additional cases:
- `TestManifestStore_Save_FencingTokenEqualOnDisk`: verifies a tie (equal
  tokens) is NOT fenced — required for the initial write where both sides are 0.
- `TestManifestStore_Save_ErrFencedIsDistinctFromErrPrecondition`: verifies
  the two sentinel errors are not aliased, since callers must distinguish them.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Clean implementation. The create-only guard inside `Save` (using the Load-returned ETag to detect existing manifest when `ifMatch=""`) is a thoughtful API decision — the Backend.Put contract treats empty ifMatch as overwrite, but the manifest layer's semantics differ. Adding the guard at the ManifestStore boundary keeps the API clean without forcing Backend impls to grow a separate "create-only" primitive.

Fencing pre-flight reuses the Load result for both the token check and the ETag lookup — single round-trip, two pieces of information. Good API design.

13 unit tests using an in-memory `memBackend` — no S3 dependency for manifest tests. The two extra tests beyond acceptance criteria (`FencingTokenEqualOnDisk` and `ErrFencedIsDistinctFromErrPrecondition`) are exactly the right edge cases to guard.

Save defaults Version=1 and overwrites UpdatedAt to time.Now().UTC() — on-disk values are authoritative, callers don't need to manage them.
