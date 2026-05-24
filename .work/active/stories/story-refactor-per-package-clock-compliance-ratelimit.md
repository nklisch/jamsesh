---
id: story-refactor-per-package-clock-compliance-ratelimit
kind: story
stage: implementing
tags: [portal, refactor, testing]
parent: feature-refactor-per-package-clock-compliance
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# ratelimit.Store: add Clock field for GC and token-bucket refill

## Brief

`internal/portal/ratelimit/store.go` calls `time.Now()` directly at three
sites — GC initialization (line 77), token-bucket refill (line 100), and
GC sweep (line 137). The package has no Clock interface yet.

This makes rate-limiter GC timing and token refill untestable without
real wall-clock waits, which is the typical motivation for the
per-package-clock-interface pattern.

## Current state

```go
// ratelimit/store.go
type Store struct {
    // ... existing fields, no clock
    lastGC time.Time
}

func NewStore(...) *Store {
    return &Store{
        // ...
        lastGC: time.Now(),
    }
}

func (s *Store) Allow(key string) bool {
    // ...
    now := time.Now()
    // ... bucket refill logic uses `now`
}

func (s *Store) maybeGC() {
    // ...
    now := time.Now()
    // ... GC sweep logic uses `now`
}
```

## Target state

```go
// ratelimit/clock.go (new file) OR add to store.go
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }
```

```go
// ratelimit/store.go
type Store struct {
    // ... existing fields
    clock  Clock
    lastGC time.Time
}

func NewStore(...) *Store {
    return NewStoreWithClock(..., realClock{})
}

func NewStoreWithClock(..., clock Clock) *Store {
    return &Store{
        // ...
        clock:  clock,
        lastGC: clock.Now(),
    }
}

func (s *Store) Allow(key string) bool {
    // ...
    now := s.clock.Now()
    // ...
}

func (s *Store) maybeGC() {
    // ...
    now := s.clock.Now()
    // ...
}
```

## Implementation notes

- Add `clock Clock` field to `Store`.
- Provide `NewStoreWithClock(..., clock Clock) *Store`. Default `NewStore`
  wraps with `realClock{}`.
- Replace all three `time.Now()` sites with `s.clock.Now()`.
- Existing tests likely use real waits. Add a new test that injects a fake
  clock and verifies GC fires after the configured interval without wall-clock
  sleep.
- Verify `cmd/portal/main.go` wires `NewStore` — no changes there if the
  default constructor preserves the production path.

## Acceptance criteria

- [ ] `ratelimit.Store` carries a `clock Clock` field.
- [ ] Constructor pair (`NewStore` + `NewStoreWithClock`).
- [ ] All three `time.Now()` sites read `s.clock.Now()`.
- [ ] At least one new test in `internal/portal/ratelimit/store_test.go`
      exercises GC or bucket refill via a fake clock — no wall-clock sleeps.
- [ ] `go build ./...` clean.
- [ ] `go test ./internal/portal/ratelimit/...` clean.

## Risk

**Low.** Self-contained package, single struct, well-scoped change.

## Rollback

`git revert` the implementation commit. `Store`'s public API gains
`NewStoreWithClock` but `NewStore` remains backwards-compatible.
