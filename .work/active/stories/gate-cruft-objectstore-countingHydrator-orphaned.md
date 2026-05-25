---
id: gate-cruft-objectstore-countingHydrator-orphaned
kind: story
stage: done
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# Defined-but-unused test helpers `countingHydrator` / `countingHydrator.Hydrate` / `newFailingHydrator` (orphaned after refactor)

## Confidence
High

## Category
dead function

## Location
`internal/portal/storage/objectstore/lifecycle_test.go:180-212`

## Evidence
```go
// countingHydrator wraps a real Hydrator and counts Hydrate calls.
type countingHydrator struct { ... }
func (c *countingHydrator) Hydrate(...) (...) { ... }

// wrappedHydrator adapts countingHydrator to look like *Hydrator for
// newTestLifecycleManager. ...
// For tests that need hydration-call counting we instrument via
// storage.CreateRepoCalled and backend state.
// For the hydration-failure test we use a nil-backend Hydrator that always fails.

func newFailingHydrator(stor storage.Service) *Hydrator { ... }
```

## Removal
`deadcode -test` flags both `countingHydrator.Hydrate` and `newFailingHydrator`. The comment block (lines 195-202) explicitly says the tests pivoted to `storage.CreateRepoCalled` and a `nil-backend Hydrator` — but `newFailingHydrator` builds an `errBackend`-based hydrator, not the documented nil-backend. Grep confirms zero call sites. Delete the `countingHydrator` type + method + orphaned-design explanatory comment + `newFailingHydrator`. Keep `errBackend` only if other code still uses it (verify before removal).

## Implementation notes

Deleted unused `countingHydrator` type + `Hydrate()` method + the orphaned-design comment block + `newFailingHydrator()` constructor from `internal/portal/storage/objectstore/lifecycle_test.go:180-212`. `errBackend` is preserved — still used by lifecycle_test.go:378-379 for the simulated-backend-failure test.

Verified: `go build ./...` clean. Affected Go tests pass (`go test ./internal/portal/playground/... ./internal/portal/storage/objectstore/...`) excluding the pre-existing `TestJoinPlaygroundSession_WithNickname_UsesIt` failure on `main` (parked as `bug-playground-join-with-nickname-returns-410-on-fresh-session`). Frontend tests pass for the two touched files (`vitest run`).
