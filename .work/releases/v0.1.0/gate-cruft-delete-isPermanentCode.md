---
id: gate-cruft-delete-isPermanentCode
kind: story
stage: done
tags: [cleanup, plugin]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# Dead helper `isPermanentCode` never called by `Classify`

## Confidence
High

## Category
dead function

## Location
`cmd/jamsesh/pusherr/classify.go:52-62`

## Evidence
```go
func isPermanentCode(code string) bool { /* checks push.* and auth.* */ }
// ...
func Classify(httpStatus int, body []byte) Result {
    // 4xx: parse body and classify by code.
    r := Result{Class: Permanent, HTTPStatus: httpStatus}
    // All 4xx are permanent ... no point retrying a client error.
    return r
}
```

## Removal
Delete `isPermanentCode`. `Classify` already buckets all 4xx as Permanent
without consulting the code; the helper is unreachable and the
surrounding doc comment explicitly notes the code-level check is
unnecessary.

## Implementation notes

- Deleted `isPermanentCode` (lines 52-62) from `cmd/jamsesh/pusherr/classify.go`.
- Removed the now-unused `strings` import (was only referenced by the deleted function).
- Trimmed the package-level doc comment: removed the stale bullet about "4xx with a recognised permanent error code" and the paragraph listing permanent error code prefixes — both described logic that lived only in the dead helper.
- Updated the package doc to accurately reflect the actual rule: "Any 4xx → Permanent".
- Build and tests pass (`go build`, `go test` — 0.001s, ok).

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Mechanical change matching the gate finding spec. Implementation notes accurately describe what was changed. Global `go build ./...` and `go test ./internal/portal/...` pass after the wave landed.
