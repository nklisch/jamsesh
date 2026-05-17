---
id: epic-cloud-native-deploy-lease-fencing-interface-and-noop
kind: story
stage: review
tags: [portal]
parent: epic-cloud-native-deploy-lease-fencing
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Lease+Fencing — Interface + NoopManager

## Scope

Define the `lease.Manager` and `lease.Handle` interfaces plus sentinel
errors. Implement `NoopManager` as the single-instance compatibility
shim — always succeeds, never blocks, Handle.Lost() never fires until
Release().

Implements **Unit 1** of `epic-cloud-native-deploy-lease-fencing`.

## Files

New:
- `internal/portal/lease/lease.go` — `Manager`, `Handle`, `ErrAlreadyHeld`
- `internal/portal/lease/noop.go` — `NoopManager`
- `internal/portal/lease/lease_test.go`

## Acceptance criteria

- [ ] `Manager` and `Handle` interfaces compile and match the spec
- [ ] `NoopManager.Acquire(ctx, sessionID)` returns a Handle with
  `FencingToken() == 0`
- [ ] `Handle.SessionID()` returns the requested id
- [ ] `Handle.Lost()` returns a channel that doesn't fire until
  `Release()` is called
- [ ] `Release()` is idempotent (safe to call multiple times)
- [ ] Unit tests cover Noop behavior + interface contract

## Notes

- `ErrAlreadyHeld` is the only sentinel; other errors wrap underlying
  causes via `%w`.
- The Noop handle should preserve the consumer-side `select` shape —
  downstream code (object-storage-sync, hydration-handoff) selects on
  Lost() in both single and clustered modes.
- `FencingToken() == 0` is the documented "no fencing required" sentinel
  for consumers that gate writes on token monotonicity.

## Implementation notes

Three new files under `internal/portal/lease/`:

- **`lease.go`** — `Manager` interface (single method: `Acquire`),
  `Handle` interface (`SessionID`, `FencingToken`, `Lost`, `Release`),
  and `ErrAlreadyHeld` sentinel. Full godoc on each method documenting
  the 0-token sentinel, the Lost() channel shape, and Release idempotency.

- **`noop.go`** — `NoopManager{}` struct satisfying `Manager`. `Acquire`
  checks `ctx.Err()` first (returns error on pre-cancelled context), then
  returns a `*noopHandle`. The unexported `noopHandle` stores the sessionID,
  a `make(chan struct{})` for `Lost()`, and a `sync.Once` that gates the
  single `close(h.lost)` on `Release()`. Second and subsequent `Release()`
  calls are swallowed by `sync.Once` — no panic, no double-close.

- **`lease_test.go`** — external package (`lease_test`). 9 test functions:
  - Compile-time interface compliance check
  - Acquire success / non-nil Handle
  - SessionID echoed correctly
  - FencingToken == 0
  - Lost() does not fire before Release()
  - Lost() closes promptly after Release()
  - Release() is idempotent (called 5×)
  - Acquire with pre-cancelled context returns error, nil Handle
  - Multiple Acquire calls for same sessionID all succeed (Noop: no exclusion)
  - Consumer `select` shape test mimicking object-storage-sync / hydration-handoff usage

All tests pass `go test -race ./internal/portal/lease/...`.
