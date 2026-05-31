---
id: e2e-handoff-githttp-push-disconnect-to-hydrated-survivor
kind: story
stage: done
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# githttp: push to freshly-hydrated survivor disconnects mid-sideband

## Brief
After the lease-takeover + non-fast-forward fixes, both handoff chaos tests
advanced to a new product failure. The survivor pod hydrates the session repo
from MinIO successfully, then a `git push` to the survivor's git smart-HTTP
endpoint fails:

```
send-pack: unexpected disconnect while reading sideband packet
fatal: the remote end hung up unexpectedly
```

The push is made DIRECTLY to the survivor pod's URL (not via the router), so
this is a server-side githttp/receive-pack defect on a pod serving a push for a
just-hydrated repo — not a routing issue.

## Suspected area / context
- `internal/portal/githttp/receive_pack.go` and the sideband / report-status
  wrapping path.
- Strongly related to the prior released bug
  `bug-receive-pack-report-status-sideband-wrapping` (v0.3.0,
  `.work/releases/v0.3.0/`) — read it for the earlier fix; this looks like a
  residual/edge case it didn't cover (the hydrated-survivor push path).
- Possibly the receive-pack subprocess output buffering / sideband framing, or
  the report-status that `Emitter.EmitForUpdates → Syncer.SyncPushPath` runs
  synchronously before the 200 (a slow/failed sync could drop the connection).

## Affects
chaos `handoff_under_pod_kill_test.go`, `handoff_under_object_storage_chaos_test.go`.
Both assert directly against the survivor (bypassing the router), so fixing this
should let them go GREEN.

## Acceptance
The push to the hydrated survivor succeeds; `TestHandoffUnderPodKill` and
`TestHandoffUnderObjectStorageChaos` pass (or advance to a further, separately-
tracked layer). Reproduce → root-cause → minimal fix → verify.

## Root cause (confirmed 2026-05-31)

Not a sideband-framing bug. The "unexpected disconnect while reading sideband
packet" is the git client's generic reaction to an **HTTP 500** received mid-RPC.
The survivor pod's container log showed the real error:

```
ERROR receive-pack: build validation repo  err="object not found"  repo=.../<session>.git
ERROR http error  code=internal  status=500  err="object not found"
```

`git send-pack` transmits a **thin pack** on every incremental push: new objects
are `REF_DELTA`-encoded against base objects the server already has but which are
NOT included in the pack. `buildValidationRepo` (`receive_pack.go`) parsed the
pushed pack with `packfile.NewParserWithStorage(scanner, memStore)` — handing the
parser **only the in-memory storer**. When go-git's parser hits a thin-pack
`REF_DELTA` whose base object isn't in the pack, it resolves it via
`storage.EncodedObject`; against `memStore` alone that returns
`plumbing.ErrObjectNotFound` → `parser.Parse()` fails → handler returns 500.

This is exactly the freshly-hydrated-survivor situation: the base objects (the
pre-kill commits 1–5) live **on disk**, hydrated from MinIO, while the 6th push's
pack is thin relative to them. (On the original holder the same code path is also
latent for any incremental push; the survivor just makes it deterministic.)

## Fix

`internal/portal/githttp/receive_pack.go` — `buildValidationRepo` now opens the
on-disk bare repo *before* parsing and hands the parser a new `parserStorer`
adapter instead of the bare `memStore`:

- **writes** (`SetEncodedObject`) go to the in-memory storer, so pushed objects
  never touch disk during validation;
- **reads** (`EncodedObject`) try memory first, then fall through to the on-disk
  repo, so external thin-pack delta bases resolve.

Newly-parsed pack objects are still validated in memory and layered over disk by
the existing `layeredStorer`; no behaviour change for the accept/reject decision
or the subprocess `git receive-pack` path. ~30 LoC, single file.

## Verification

- Unit regression: `TestReceivePack_ThinPackAgainstOnDiskBase` (new, in
  `receive_pack_test.go`) — pushes a large file, then pushes a small edit from a
  fresh clone so git emits a thin pack deltified against the on-disk base. PASS
  with the fix. With the disk fallback disabled it reproduces the **exact** story
  symptom (`send-pack: unexpected disconnect while reading sideband packet`),
  confirming it is a genuine regression test.
- `go test ./internal/portal/githttp/ ./internal/portal/postreceive/
  ./internal/portal/storage/objectstore/` — all green.
- e2e (after `make test-portal-image`):
  `go test -p 1 ./chaos/ -run 'TestHandoffUnderPodKill|TestHandoffUnderObjectStorageChaos'`
  → both **PASS** (pod-kill 4.74s, obj-storage-chaos 29.48s), zero data loss,
  no router 502 (handoff tests address the survivor directly).
