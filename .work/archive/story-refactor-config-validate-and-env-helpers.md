---
id: story-refactor-config-validate-and-env-helpers
kind: story
stage: done
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

## Implementation notes

**Files changed:**
- `internal/portal/config/secrets.go` — added `mustBePositive`, `mustBeNonNegative`, `readEnvString`, `readEnvInt`, `readEnvInt64`, `readEnvDuration` helpers
- `internal/portal/config/config.go` — reduced from 858 → 685 lines (−173 LoC)

**Placement decision:** New helpers added to `secrets.go` to co-locate all
env-reading primitives in one place, consistent with the existing `readEnvOrFile`.

**validate() refactor:** The 9 "must be positive" checks are now driven by a
small anonymous struct slice iterated with `mustBePositive`. `HydrationCacheMaxBytes`
uses the sibling `mustBeNonNegative` (int64) to preserve the distinct "zero means
unlimited" semantics.

**applyEnv() refactor:** All sub-functions (`applyLeaseEnv`, `applyObjectStorageEnv`,
`applyHydrationEnv`, `applyGitEnv`, `applyDBEnv`, `applyAuthRateLimitEnv`,
`applyMetricsEnv`, `applyAPIBodyLimitEnv`, `applyPlaygroundEnv`) were inlined into
a single flat `applyEnv()` using the new helpers. Only `applyEmailEnv` and
`applyOAuthEnv` remain as separate functions because they have error return paths
from `readEnvOrFile` secrets.

**Bool knobs kept inline:** `OBJECT_STORAGE_PATH_STYLE`, `AUTH_RATE_LIMIT_ENABLED`,
and `PLAYGROUND_ENABLED` each have distinct truthiness semantics and are kept as
inline `if` blocks with comments explaining the divergence. A shared bool parser
would mask these differences.

**JAMSESH_API_BODY_LIMIT_BYTES:** Kept inline because it has an extra `n > 0`
guard that the generic `readEnvInt64` doesn't carry.

**Pre-existing build failures:** `cmd/portal`, `internal/portal/router`, and
`internal/portal/server` test packages fail to build due to an unrelated
in-flight router refactor story. These failures pre-date this commit.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The `if v, err := readEnvOrFile(...); err != nil { return err } else if v != "" { ... }` shape may trigger `revive: indent-error-flow` linting. Cosmetic.

**Notes**: All 53 JAMSESH_* env vars preserved 1:1 (verified by extracting var names from both sides of the diff). `validate()` collapsed into table-driven `mustBePositive` loop with identical error wording. `HydrationCacheMaxBytes` correctly routed to `mustBeNonNegative(int64)` preserving the "zero or positive" semantic. Three bool knobs (`OBJECT_STORAGE_PATH_STYLE`, `AUTH_RATE_LIMIT_ENABLED`, `PLAYGROUND_ENABLED`) kept inline with documented distinct truthiness rules. `API_BODY_LIMIT_BYTES` kept inline for `n > 0` guard. Secret knobs still flow through `readEnvOrFile`. `go test ./internal/portal/config/...` clean.
