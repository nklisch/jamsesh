---
id: e2e-handoff-githttp-push-disconnect-to-hydrated-survivor
kind: story
stage: implementing
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
