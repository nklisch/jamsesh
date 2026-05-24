---
id: feature-refactor-per-package-clock-compliance
kind: feature
stage: done
tags: [portal, refactor, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Per-package Clock interface compliance across remaining packages

## Brief

The project's documented `per-package-clock-interface` pattern
(`.claude/skills/patterns/per-package-clock-interface.md`) is observed
in `sessions/`, `tokens/`, `comments/`, `accounts/`, `automerger/`,
`finalize/`, `playground/`, `wsgateway/`, `mcpendpoint/`, and `events/`,
but several packages still call `time.Now()` directly. The result is
that timing-dependent logic in those packages is not exercisable by
`*testclock.AdvanceableClock` — tests have to manipulate system time or
use real wall-clock timeouts.

Surfaced by a discovery-mode `/agile-workflow:refactor-design` scan.

## Call sites missing clock injection

| Package / file | Line(s) | Use |
|---|---|---|
| `internal/portal/auth/oauth.go` | 110 | OAuth state-row expiry check |
| `internal/portal/auth/provision.go` | 43 | `FindOrProvision` wrapper passes `time.Now().UTC()` to `FindOrProvisionAt` |
| `internal/portal/storage/objectstore/manifest.go` | 178 | `m.UpdatedAt = time.Now().UTC()` inside `Save` |
| `internal/portal/storage/objectstore/lifecycle.go` | 99, 180, 337 | Session entry touch / acquire / eviction |
| `internal/portal/ratelimit/store.go` | 77, 100, 137 | GC init, token-bucket refill, GC sweep |
| `internal/portal/lease/retention.go` | 29 | Retention cutoff |

## Out of scope

`internal/portal/auth/slug.go:66` also reads `time.Now().UnixNano()`,
but as a PRNG seed. Replacing it changes RNG behavior — tracked
separately under a non-`[refactor]` story so feature-design picks the
classification.

## Design questions for feature-design

- Wiring: each package needs a `Clock interface{ Now() time.Time }`
  field on the relevant struct and constructor wiring. Should
  `objectstore` share one clock across `Manifest`, `Sync`, and
  `Lifecycle`, or per-component clocks like elsewhere in the codebase?
- For functions that take no struct receiver (`lease.RunRetention`,
  `auth.FindOrProvision`), the choice is between adding a `Clock`
  parameter (changes call sites) or threading it via context. Pattern
  reference is `FindOrProvisionAt` — explicit time parameter.
- Test posture: which existing tests can drop their wall-clock waits
  once injection lands?

## Acceptance criteria (target)

- Every `time.Now()` call in the listed call sites is replaced by an
  injected `Clock` field or parameter.
- The pattern matches `.claude/skills/patterns/per-package-clock-interface.md`
  — package-local `Clock` interface with a `realClock{}` default.
- Existing tests still pass; at least one new test per package
  exercises clock advancement to lock in the contract.
- `go build ./...` and `go test ./...` clean.

## Notes

Behavior-preserving — production wiring uses `realClock{}` so calls
to `Now()` return the same value as before. The intent is testability.

## Refactor Overview

The 7 call sites listed in the brief partition cleanly across 4 package
boundaries. Each boundary lands as one child story; all four are
independent (no `depends_on` between them) so they run as a single
parallel wave.

Two patterns are deliberately mixed across the children:
- **Struct-field Clock** (matches `events.Log`, `tokens.Service`, etc.) —
  used when the touched code is on a long-lived service:
  `auth.OAuthHandler`, `objectstore.LifecycleManager`, `ratelimit.Store`.
- **Parameter-passing `now time.Time`** (matches `auth.FindOrProvisionAt`) —
  used when the touched code is a free function or per-call value type:
  `lease.RunRetention`, `objectstore.Manifest.Save`.

Both `auth` and `storage` already define a package-local `Clock` interface
that callers can lean on; `objectstore`, `ratelimit`, and `lease` need
fresh package-local definitions (or, for `lease`, just the parameter).

## Refactor Steps

### Step 1: auth (OAuthHandler + provision)
**Priority**: Medium  **Risk**: Low
**Files**: `internal/portal/auth/oauth.go`, `internal/portal/auth/provision.go`
**Story**: `story-refactor-per-package-clock-compliance-auth`

Route `OAuthHandler.OauthCallback`'s state-expiry check through the
existing `auth.Clock`. Update clock-aware callers of `FindOrProvision`
to call `FindOrProvisionAt` directly with their clock's `Now()`.

### Step 2: storage/objectstore (LifecycleManager + Manifest.Save)
**Priority**: High  **Risk**: Medium
**Files**: `internal/portal/storage/objectstore/lifecycle.go`,
`internal/portal/storage/objectstore/manifest.go`,
`internal/portal/storage/objectstore/clock.go` (new)
**Story**: `story-refactor-per-package-clock-compliance-objectstore`

Define `objectstore.Clock` interface + `realClock`. `LifecycleManager`
gets a clock field (3 call sites); `Manifest.Save` takes a `now time.Time`
parameter (1 call site). Caller surface: `Syncer.SyncPushPath` is the
primary `Save` caller and must thread the clock through.

### Step 3: ratelimit.Store
**Priority**: Medium  **Risk**: Low
**Files**: `internal/portal/ratelimit/store.go`
**Story**: `story-refactor-per-package-clock-compliance-ratelimit`

Add `Clock` interface + `realClock` (in `store.go` or a sibling `clock.go`).
`Store` gets a clock field (3 sites); constructor pair
`NewStore` + `NewStoreWithClock`.

### Step 4: lease.RunRetention
**Priority**: Low  **Risk**: Low
**Files**: `internal/portal/lease/retention.go`
**Story**: `story-refactor-per-package-clock-compliance-lease`

`RunRetention` accepts an explicit `now time.Time` parameter — matches
the `FindOrProvisionAt` shape. No package-level Clock interface needed;
the function is small enough that the parameter form is cleaner.

## Implementation Order

All four stories run in parallel (Wave 1). No `depends_on` between them.

## Bonus findings (out of scope; logged for follow-up)

The discovery scan surfaced a few related sites that this feature
deliberately does NOT touch:

- `internal/portal/automerger/outcomes.go:66` — direct `time.Now().UTC()`
  despite the package having a `realClock` at line 33. Looks like a missed
  substitution; small follow-up story candidate.
- `internal/portal/githttp/receive_pack.go:337` — direct `time.Now().UTC()`;
  the package has no `Clock` pattern yet. Larger follow-up — receive-pack
  is on the hot path for git operations and would benefit from injectable
  time for testing.
- `internal/portal/lease/postgres.go:177` — direct `time.Now()` in the
  lease driver, separate from `retention.go`. Follow-up.
- `internal/portal/auth/slug.go:66` — `time.Now().UnixNano()` PRNG seed —
  intentionally out of scope (behavior-changing).

## Implementation summary (orchestrator)

All 4 child stories advanced to `stage: review`:

- `story-refactor-per-package-clock-compliance-ratelimit` — `ratelimit.Store` gained `clock Clock` field + constructor pair; 3 `time.Now()` sites routed; 2 new fake-clock tests (GC + bucket refill) with no wall-clock sleep
- `story-refactor-per-package-clock-compliance-lease` — `RunRetention` accepts explicit `now time.Time`; cutoff regression test with no sleep
- `story-refactor-per-package-clock-compliance-auth` — `OAuthHandler` clock field + `NewOAuthHandlerWithClock`; expired-state branch now driven by fake clock; `FindOrProvision` callers routed through `FindOrProvisionAt(..., h.clock.Now())`
- `story-refactor-per-package-clock-compliance-objectstore` — new `objectstore.Clock` interface; `LifecycleManager` clock field via `m.now()` accessor; `ManifestStore.Save` parameter form (matches `FindOrProvisionAt`); 2 new fake-clock tests

**Verification**: `go build ./...` clean, `go test ./...` clean across all 57 packages.

## Review (2026-05-23)

**Verdict**: Approve — feature delivered as briefed.

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: All child stories approved individually. Aggregate review: design decomposition realized end-to-end, no cross-cutting deviations beyond what's documented in the implementation summary, no foundation-doc drift, no API breakage beyond intra-`internal/` boundaries (all callers updated in-tree).
