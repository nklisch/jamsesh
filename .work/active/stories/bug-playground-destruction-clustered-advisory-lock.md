---
id: bug-playground-destruction-clustered-advisory-lock
kind: story
stage: review
tags: [portal, playground, clustered, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
---

# Playground destruction worker: missing PG advisory lock for clustered mode

## Origin

Filed as a review finding (important) on
`story-epic-ephemeral-playground-session-lifecycle-destruction`. The
parent feature design (`feature-epic-ephemeral-playground-session-lifecycle`
Risks section) and the story's "Notes for the implementing agent"
explicitly called for the destruction routine to wrap per-session work in a
PG advisory lock acquired via the existing `LeaseManager` infrastructure
(NoopManager under single-instance, real PG advisory lock under
`JAMSESH_DEPLOY_MODE=clustered`). The implementation in
`internal/portal/playground/destruction.go` does not wire this lock.

## Why it matters

Under clustered mode, multiple portal pods all run the destruction worker
goroutine. When two pods see the same expired session in the same tick they
will both run the 8-step cascade against it. The implementation is largely
idempotent and absorbs most race outcomes:

- Step 3 RecordTombstone uses ON CONFLICT DO NOTHING.
- Step 6 DeleteSession returns ErrNotFound on the second deleter, which is
  tolerated explicitly in destruction.go.
- Step 7 DeleteAccountsByIDs is harmless when the rows are already gone.
- Step 8 RemoveRepo wraps os.RemoveAll which is idempotent.

The residual race risks are:

- Duplicate log lines and duplicate tombstone summary computation per
  expired session per sweep — wastes DB reads but is not corruption.
- Two pods both running Step 4 RevokeBearersForSession concurrently — both
  succeed (UPDATE … WHERE revoked_at IS NULL); benign.
- Steps 1-2 (count members, list anon IDs) executed twice with the same
  result; benign as long as Step 7 sees the same anonIDs set both times,
  which it does because no other code path mutates session_members between
  ListExpiredPlaygroundSessions and DeleteSession.

So this is not a correctness bug today, but it leaves the rolling-foundation
principle in soft drift: the design promises an advisory lock and the code
doesn't have one. Clustered-mode deployments will accumulate noise in logs
and waste DB roundtrips; future tweaks (e.g. a non-idempotent step added to
the cascade, or per-destroy metrics emission) would silently introduce
correctness bugs without the lock.

## Acceptance criteria

- [x] `Destruction.Destroy` wraps the cascade in a per-session advisory
      lock acquired via the existing `LeaseManager` interface.
- [x] Lock key is deterministic from the session ID (e.g. hash of the
      session ID into a Postgres int8 for pg_advisory_lock).
- [x] Under NoopManager (single-instance default) the lock is a no-op and
      the cascade runs as today.
- [x] If the lock cannot be acquired (another pod holds it), `Destroy`
      returns nil immediately — the other pod owns this destruction; this
      pod will retry on the next sweep.
- [x] Test: stub a LeaseManager that fails the second acquire; verify the
      second concurrent Destroy is a no-op.

## Notes

- The LeaseManager interface and NoopManager live alongside the existing
  cross-pod coordination primitives — see the parent feature body's
  Risks > Clustered-mode interaction for the design hook.
- Low priority because single-instance is the default deploy mode; no
  production user is at risk today. Bump if/when a clustered deployment
  is provisioned.

## Implementation notes

**LeaseManager API used:** `lease.Manager.Acquire(ctx, sessionID)`. The
existing `Manager` interface already exposes a non-blocking try-acquire:
`Acquire` returns `lease.ErrAlreadyHeld` immediately when another pod holds
the lock (backed by `pg_try_advisory_lock` in `PostgresManager`). No
interface extension was needed — the API fit exactly.

**Lock key / session ID hashing:** The `Destroy` method passes `sess.ID`
(the plain string session identifier) directly as the `sessionID` argument
to `Acquire`. In the `PostgresManager` this becomes the argument to
`hashtext($1)`, mapping the session ID string to a Postgres int32 advisory
lock key. In `NoopManager` the string key is used in an in-process map. No
additional hashing is performed in the `playground` package — the
`LeaseManager` owns that concern.

**Wiring change in main.go:** The `playground.Worker` struct gained a
`Leases lease.Manager` field. `Worker.Run()` passes `w.Leases` through to
the `Destruction` it constructs, which in turn calls `d.leases()` (a helper
that falls back to `lease.NoopManager{}` when the field is nil). In
`cmd/portal/main.go`, the already-constructed `leaseMgr` is now set on the
`destructionWorker` struct literal (one-line change).

**Contention test shape:** `TestDestruction_AdvisoryLock_SecondDestroyIsNoOp`
uses `stubLeaseManager` — a mutex-backed in-process implementation of
`lease.Manager` that enforces single-holder semantics (first `Acquire`
succeeds, second returns `ErrAlreadyHeld` while first is held, succeeds
again after `Release`). The test pre-acquires the lock to simulate the
winner pod, calls `loser.Destroy` while the lock is held (asserting nil
return and that the session row still exists), then releases the lock and
confirms the winner's `Destroy` runs the full cascade (session gone, tombstone
present).
