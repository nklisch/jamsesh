# Pattern: Per-Package Clock Interface

Each package that needs a time source defines its own one-method
`Clock interface { Now() time.Time }` (plus an unexported `realClock{}`
fallback), rather than importing a shared `Clock` type from a sibling
package. The same concrete `*testclock.AdvanceableClock` satisfies all
of them by structural typing.

## Rationale

Avoids cross-package import coupling (auth, comments, sessions,
finalize, events, storage, etc. don't take a dependency on each other
for a one-method interface) while still letting one concrete test-clock
pump advance every package in lockstep. Co-located comments throughout
the codebase document this verbatim: "Mirrors auth.Clock and
tokens.Clock so a single *testclock.AdvanceableClock satisfies all of
them. Per-package types avoid cross-package import coupling — structural
typing carries the 'advance once, move everywhere' property".

## Examples

### Example 1: comments package

**File**: `internal/portal/comments/service.go:33`

```go
// Clock is an injectable time source. Mirrors auth.Clock and tokens.Clock so a
// single *testclock.AdvanceableClock satisfies all of them. Per-package types
// avoid cross-package import coupling — structural typing carries the
// "advance once, move everywhere" property.
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }
```

### Example 2: sessions package (identical shape)

**File**: `internal/portal/sessions/clock.go:9`

```go
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }
```

### Example 3: storage package

**File**: `internal/portal/storage/service.go:82` — same shape, plus a
`NewWithClock(rootDir, store, clock)` constructor for tests in addition
to the default `New(...)` which calls `NewWithClock(..., realClock{})`.

Replicated identically across 14 packages total:
`internal/portal/{accounts, auth, automerger, comments, events, finalize,
mcpendpoint, playground, ratelimit, sessions, storage,
storage/objectstore, tokens, wsgateway}`.

## When to Use

- A portal-internal package needs the current UTC time inside a method
  that should be deterministic in tests.
- The package is one of the existing 14 that already exports a Clock —
  extend that local interface.

## When NOT to Use

- A leaf helper that doesn't need to be mocked — just call
  `time.Now().UTC()` inline.
- Cross-binary or cross-module boundaries where you actually want a
  shared symbol (use `testclock.AdvanceableClock` directly).

## Common Violations

- Importing `comments.Clock` from `sessions` to reuse the interface —
  creates the exact cross-package coupling this pattern was designed to
  avoid.
- Storing a `*time.Time` field instead of a `Clock`, which makes test
  advancement require a per-call setter.
