---
id: story-refactor-config-validate-and-env-helpers
kind: story
stage: implementing
tags: [portal, refactor]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Extract mustBePositive + readEnvString helpers in portal config

## Brief

`internal/portal/config/config.go` (858 lines) contains two repetitive
patterns that each fire ~15+ times:

**1. `validate()` (lines 489-551)** — 18 consecutive blocks of
`if c.FieldName <= 0 { return fmt.Errorf("config: <field> must be positive (got %d)", c.FieldName) }`

**2. `applyEnv()` and its sub-functions (lines 556-770)** — `if v := os.Getenv("..."); v != "" { c.Field = v }` and the
parsed-integer variant `if v := os.Getenv("..."); v != "" { n, err := strconv.Atoi(v); ... }`

## Current state

Each call site is its own block; the noise hides the actual interesting
defaults and the few config knobs that have non-trivial validation
(e.g. choice strings, paths, bounded ranges).

## Target state

Two unexported helpers in the same file:

```go
func mustBePositive(name string, v int) error {
    if v <= 0 {
        return fmt.Errorf("config: %s must be positive (got %d)", name, v)
    }
    return nil
}

func readEnvString(key string, dst *string)            { ... }
func readEnvInt(key string, dst *int) error            { ... }
func readEnvDuration(key string, dst *time.Duration) error { ... }
func readEnvBool(key string, dst *bool) error          { ... }
```

`validate()` collapses into a sequence of `if err := mustBePositive(...); err != nil { return err }` lines OR a small accumulator loop.

`applyEnv()` and its sub-functions become a flat list of
`readEnvX(...)` calls with errors checked once at the bottom.

## Acceptance criteria

- `internal/portal/config/config.go` LoC reduced by at least 100.
- `validate()` ≤ ~30 LoC.
- All existing `internal/portal/config/...` tests pass.
- `go build ./...` clean.
- Behavior-preserving: identical error messages for identical inputs
  (the helper carries the field name forward).

## Notes

Behavior-preserving — error message wording and env-var precedence are
preserved. The file currently has `secrets.go` with a `readEnvOrFile`
helper already; this story extends that file with the simpler
`readEnvX` helpers and applies them at every call site in `config.go`.
