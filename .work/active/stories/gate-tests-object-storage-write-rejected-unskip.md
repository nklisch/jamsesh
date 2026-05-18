---
id: gate-tests-object-storage-write-rejected-unskip
kind: story
stage: implementing
tags: [testing, infra, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# `TestObjectStorageWriteRejected` PATH B (e2e) remains skipped post-fix

## Priority
High

## Spec reference
Item: `object-storage-write-rejected-silent-acceptance` (archived, but
the skip was left in place).
Acceptance criterion: RPO=0 is a deployment-level invariant that
warrants e2e coverage; the unit test alone is insufficient.

## Gap type
test-integrity (skip-without-replacement at e2e tier).

## Suggested test
Remove the `t.Skip` at
`tests/e2e/failure/object_storage_write_rejected_test.go:245` and turn
it into an explicit failure assertion:

```go
// After cluster start + push: assert git push exited non-zero AND bucket is empty.
// No t.Skip: a 2xx push to a missing bucket is now an unambiguous test failure.
```

## Test location (suggested)
`tests/e2e/failure/object_storage_write_rejected_test.go`
