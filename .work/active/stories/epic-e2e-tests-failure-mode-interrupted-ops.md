---
id: epic-e2e-tests-failure-mode-interrupted-ops
kind: story
stage: implementing
tags: [e2e-test, testing]
parent: epic-e2e-tests-failure-mode
depends_on: [epic-e2e-tests-failure-mode-rest-validation]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Failure — Interrupted operations

## Scope

One Go spec `tests/e2e/failure/interrupted_ops_test.go` covering
operations that get interrupted mid-flight: pushes, finalize locks,
magic-link TTLs, and WebSocket connections.

### Categories covered

- Push interrupted mid-pack (client connection severed during
  `git-receive-pack`)
- Finalize lock acquired then the holder process killed (lock should
  expire / be reclaimable)
- Magic-link request submitted but exchange skipped past the 15-minute
  TTL
- WebSocket connection drop mid-event-burst (client reconnects, events
  replay correctly)

## Files to create / modify

- `tests/e2e/failure/interrupted_ops_test.go` (NEW) — main spec

## Acceptance criteria

- [ ] Spec is green via `cd tests/e2e && go test ./failure/ -run
      TestInterruptedOps -v`
- [ ] Each subtest exercises a real interruption (no mocked failure
      points)
- [ ] Each subtest's invariant is stated in plain English
- [ ] Assertions are on user-visible outcomes: pack rejection on
      retry, lock reacquisition, magic-link 401 with
      `auth.expired_token`, WebSocket replay payload shape

## Notes for the implementer

- Push interruption: use a `context.WithTimeout` of ~100ms when
  posting to `/git-receive-pack`, then retry with a fresh client.
  Assert the second push either fully succeeds (idempotent recovery)
  or fails cleanly with a documented error — match whichever the
  portal's behavior actually is.
- Finalize lock killed: acquire the lock with one bearer token, then
  let the lock TTL expire (or `Delete` if a programmatic release
  exists). Another caller should be able to re-acquire.
- Magic-link TTL: this requires advancing the portal's clock by 16
  minutes. Either use libfaketime (heavy) or directly tamper with the
  database (cheap but invasive). For now: skip past the TTL using a
  manual `sleep` — but skip the test under `-short` to keep dev loops
  fast.

  Alternative: add a test-only `/test/clock-advance` endpoint behind a
  build tag to the portal. That's a separate testability story —
  file as backlog if implementing inline is too invasive.

- WebSocket drop: connect via the WS client (from
  `tests/e2e/fixtures/wsclient/` — to be authored by
  session-lifecycle), break the connection mid-stream, reconnect with
  the same cursor, assert the events that arrived between disconnect
  and reconnect are delivered on the replay.

  If `session-lifecycle` hasn't landed yet, defer the WS subtest and
  file a follow-on dependency.

## Subtest checklist

- [ ] Push timeout → retry succeeds or fails cleanly with documented
      error
- [ ] Finalize lock TTL → reclaimable by another caller after expiry
- [ ] Magic-link 16min skip → exchange returns 401 `auth.expired_token`
      (skip under `-short`)
- [ ] WS drop → reconnect replays missed events (defer if
      session-lifecycle not landed)
