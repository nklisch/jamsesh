---
id: portal-migration-advisory-lock-blocks-startup-indefinitely
kind: story
stage: backlog
tags: [portal, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# Portal startup hangs indefinitely on `pg_advisory_lock` when a prior pod's migration lock lingers

## Summary
In clustered mode, `db.Open` serialises schema migrations across pods with a
**blocking, un-timed** session-level advisory lock:

`internal/db/migrate.go` — `withMigrationLock`:
```go
db.ExecContext(ctx, "SELECT pg_advisory_lock($1)", jamseshMigrationLockKey)
```

`pg_advisory_lock` blocks **forever** until the lock is granted. If a previous
pod held this lock and its Postgres backend connection has not yet been reaped
(e.g. the pod was SIGKILLed / OOM-killed / crashed mid-migration, or a rolling
deploy stopped it ungracefully), the new pod blocks inside `db.Open` — before it
binds its HTTP listener — and never becomes ready. `/healthz` never returns 200,
so orchestrators see the pod as perpetually unhealthy until Postgres eventually
reaps the dead session (which can take minutes, governed by TCP keepalive /
`idle_in_transaction_session_timeout`, not by the portal).

The doc comment on `withMigrationLock` claims "if the process dies or the
connection is dropped mid-migration, Postgres releases the lock automatically so
the next caller can proceed. There is no risk of permanent lock starvation."
That is only true *once Postgres notices the connection is gone* — which is not
prompt for an abruptly-killed peer. Between the kill and the reap, the next pod
is wedged.

## How it was found
Found by **code reading** while investigating intermittent portal-startup
readiness stalls in the e2e fuzz suite (`tests/e2e/fuzz/`, feature
`e2e-cloud-native-multipod-suite-red-fuzz`). Each fuzz seed boots a "bootstrap"
1-pod cluster (runs migrations, takes the lock) and a fresh "hot" 1-pod cluster
against the **same** Postgres; the hot pod occasionally hung at startup with only
`{"msg":"portal starting"}` in its logs (the next startup step,
`lease manager configured`, never appeared — localising the hang to `db.Open`,
whose only blocking call is `pg_advisory_lock`).

NOTE ON SEVERITY / REPRODUCIBILITY: in the fuzz suite this is **not reliably
deterministic** — the same seed both hangs (under heavy concurrent Docker load
on the shared host) and passes cleanly (~1.8s) when the host has resources. The
fuzz reds were ultimately attributable to host-level container-startup
contention, not a guaranteed lock deadlock. This item captures the underlying
**latent production weakness** the investigation exposed: the migration-lock
acquire has no timeout and ignores the startup deadline, so a genuinely lingering
lock (crash / OOM-kill / ungraceful rolling-deploy stop of a peer that held it)
*would* wedge a replacement pod's startup until Postgres reaps the dead session.
That is a real availability gap worth fixing independent of the e2e flakiness.

## Why this is a product bug, not test debt
The portal's startup path has no bound on how long it will wait for the
migration lock. A real clustered deploy (rolling restart, crash-loop, node
eviction) can leave the lock held by a dead-but-not-yet-reaped session, wedging
the replacement pod's startup. That is a production availability gap, not a test
artifact — the fuzz harness merely reproduces the same kill→restart race
reliably.

## Suggested fix (design needed — deep arc, out of scope for the fuzz feature)
Bound the wait and fail fast / retry instead of blocking forever. Options:
- Set `lock_timeout` (or `statement_timeout`) on the migration connection before
  `pg_advisory_lock`, so the acquire fails after N seconds; then retry with
  backoff up to a deadline, surfacing a clear "could not acquire migration lock"
  error rather than an invisible hang.
- Or use `pg_try_advisory_lock` in a bounded retry loop with backoff.
- Either way, the acquire must respect the passed `ctx` deadline (today the
  blocking call ignores any practical timeout because the startup `ctx` has
  none).
- Audit the unlock: `withMigrationLock` `defer`s `pg_advisory_unlock` on a fresh
  `context.Background()` against the same `*sql.DB` pool. A session-level
  advisory lock can only be released by the *connection* that took it; if the
  pool hands the unlock a different connection it is a silent no-op and the lock
  only releases when the migration `*sql.DB` is closed. The current code closes
  that `*sql.DB` right after, so it works today, but it is fragile — make the
  acquire+unlock run on a single pinned `*sql.Conn`.

## Affected code
- `internal/db/migrate.go` — `withMigrationLock`
- `internal/db/connect.go` — `Open` (postgres branch calls `withMigrationLock`)
