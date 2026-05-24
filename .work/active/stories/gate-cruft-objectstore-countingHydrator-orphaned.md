---
id: gate-cruft-objectstore-countingHydrator-orphaned
kind: story
stage: implementing
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
