---
id: gate-cruft-delete-isPermanentCode
kind: story
stage: implementing
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
