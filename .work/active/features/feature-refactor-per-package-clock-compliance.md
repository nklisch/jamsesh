---
id: feature-refactor-per-package-clock-compliance
kind: feature
stage: drafting
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
