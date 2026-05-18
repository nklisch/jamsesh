---
id: epic-cloud-native-deploy-lease-fencing-postgres
kind: story
stage: done
tags: [portal]
parent: epic-cloud-native-deploy-lease-fencing
depends_on: [epic-cloud-native-deploy-lease-fencing-interface-and-noop, epic-cloud-native-deploy-lease-fencing-schema]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Lease+Fencing — Postgres lease manager

## Scope

`PostgresManager` implements `lease.Manager` using a dedicated `*sql.Conn`
per active lease, with `pg_try_advisory_lock(hashtext(session_id))` as
the mutex, fencing token from the sequence, and a heartbeat goroutine
that detects connection loss and fires `Handle.Lost()`.

Implements **Unit 3** of `epic-cloud-native-deploy-lease-fencing`.

## Files

New:
- `internal/portal/lease/postgres.go` — `PostgresManager` + `pgHandle`
- `internal/portal/lease/postgres_test.go` — integration, gated on
  `JAMSESH_TEST_PG_DSN`

## Acquire sequence (per parent feature design)

1. `db.Conn(ctx)` to dedicate a `*sql.Conn` for this lease's lifetime
2. `SELECT pg_try_advisory_lock(hashtext($1))` — false → release conn,
   return `ErrAlreadyHeld`
3. `SELECT nextval('jamsesh_lease_fencing_tokens')`
4. `InsertLease` with podID, token, acquired_at
5. **Collision defensive check**: SELECT the leases row by session_id;
   if its pod_id is a DIFFERENT pod AND released_at is NULL, that's a
   hashtext collision — log warning, release lock, return ErrAlreadyHeld
6. Spawn heartbeat goroutine (PingContext every HeartbeatInterval)
7. Return Handle

## Release sequence

1. Stop heartbeat goroutine
2. `SELECT pg_advisory_unlock($1)`
3. `MarkLeaseReleased` (set released_at = now())
4. Close `*sql.Conn` (returns to pool)

## Acceptance criteria

- [ ] Acquire on a free session succeeds; returned Handle has
  monotonically-increasing FencingToken across calls
- [ ] Second Acquire from another PG session returns `ErrAlreadyHeld`
- [ ] `Handle.Lost()` closes when the underlying PG conn drops (test:
  another PG session runs `pg_terminate_backend(pid)` on the holder)
- [ ] Release frees the conn (visible in pgxpool stats) and sets
  released_at
- [ ] Release-after-Lost is idempotent (no error, no double-unlock attempt)
- [ ] Heartbeat keeps the lease alive across natural idle (sleep > heartbeat
  interval, verify Lost() hasn't fired)
- [ ] Collision check: simulate a row with mismatched pod_id and verify
  Acquire returns ErrAlreadyHeld
- [ ] Integration tests gated on `JAMSESH_TEST_PG_DSN`; skip cleanly without

## Notes

- The dedicated `*sql.Conn` is the critical correctness primitive — PG
  advisory locks are session-scoped, so lock/migrate/release must all
  land on the same PG session. This is the same lesson from the
  db-pool-and-lock story; the backlog item `graceful-shutdown-shutdownstart-race`
  documents a similar concern in a different context.
- Heartbeat interval: 10s default; configurable via `HeartbeatInterval` field.
  Failures (any error from PingContext) close Lost(). Tune via metrics
  if false-positives observed.
- PodID: read from `HOSTNAME` env var (k8s default) or generated UUID at
  pod startup. Stored in `leases.pod_id` for observability.

## Implementation notes

- `PostgresManager` in `internal/portal/lease/postgres.go`. `pgHandle` is
  unexported; all Handle interface methods are value-safe behind `sync.Once`.
- Acquire sequence exactly matches the spec: `db.Conn(ctx)` → advisory lock →
  fencing token via `Store.IssueLeaseFencingToken` → `InsertLease` upsert →
  collision check (pod_id mismatch + released_at IS NULL) → heartbeat goroutine.
- Release sequence: close `done` channel → `pg_advisory_unlock` (best-effort) →
  `MarkLeaseReleased` (best-effort) → `conn.Close()`. All wrapped in `sync.Once`.
- Heartbeat goroutine selects on a `done` channel (closed by Release) and a
  ticker; any `PingContext` error closes `Lost()` and exits. No goroutine leak.
- Integration tests in `internal/portal/lease/postgres_test.go` are gated on
  `JAMSESH_TEST_PG_DSN`; skip cleanly when absent. Tests cover: acquire success,
  monotonic fencing tokens, conflict (ErrAlreadyHeld), Lost fires on backend
  terminate, Release idempotency, heartbeat keeps lease alive, Release-after-Lost
  idempotency, and the collision defensive check.
- `go test -race ./internal/portal/lease/...` passes (unit tests only without
  PG DSN). Full `go build ./...` clean.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: This is the implementation where the dedicated `*sql.Conn` pattern is critical for correctness — and it's done right. `m.DB.Conn(ctx)` checks out a connection at step 1; all subsequent operations (advisory lock, fencing token issue, insert, collision check, heartbeat ping, release unlock) execute on that conn so they all land on the same PG session. The collision defensive check (`pod_id` mismatch + `released_at IS NULL` → ErrAlreadyHeld) is sensibly placed AFTER the advisory lock succeeds but BEFORE the heartbeat goroutine spawns — minimal cleanup needed if collision detected.

`pgHandle.Release` correctly stops the heartbeat (close `done` channel) BEFORE running the unlock + MarkLeaseReleased + conn.Close. The `sync.Once` makes Release idempotent. Heartbeat goroutine selects on `done` AND ticker, exits on ctx error OR ping failure (closing Lost()).

Tests cover the 8 critical scenarios per design including the `pg_terminate_backend` simulation for Lost() and the synthetic-row insertion for the collision-detection path. All gated on `JAMSESH_TEST_PG_DSN` and skip cleanly without.

This implementation specifically addresses the connection-reuse concern flagged during the db-pool-and-lock review nits — `db.Conn(ctx)` is the defensive pattern that the migration lock could optionally adopt.
