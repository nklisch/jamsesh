---
id: gate-tests-object-storage-write-rejected-unskip
kind: story
stage: done
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

## Implementation notes

Removed the `t.Skip(...)` at PATH B (lines 245–254 in the original) in
`tests/e2e/failure/object_storage_write_rejected_test.go` and replaced it with
a `t.Fatalf(...)` assertion.

### What the old skip said
The `t.Skip` fired when `git push` exited 0 (2xx), documenting that the portal
had silently swallowed the `NoSuchBucket` error — an RPO=0 violation at the
time the skip was written. It served as the audit trail for the bug.

### What the new assertion says
`t.Fatalf` fires on the same condition (`pushErr == nil`), but now treats it as
an **unambiguous test failure** rather than a known-acceptable gap. The message
names the invariant, explains why 2xx is wrong, and explicitly instructs
maintainers not to re-add a `t.Skip`. The fix
(`object-storage-write-rejected-silent-acceptance`) ensures `EmitForUpdates`
errors are propagated as HTTP 500 before any response bytes are committed, so
this path must not be reachable in a correct build.

### No fixture changes needed
The `minio.ListObjects` and `writeRejectedAssertBucketEmpty` helpers were
already in place. No new fixture methods were required.

### Build verification
`go build -tags e2e ./...` from `tests/e2e/` — clean, no errors.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Test-integrity restoration verified. t.Skip removed at PATH B; t.Fatalf with clear invariant message replaces it. Message instructs maintainers not to re-add t.Skip. Build clean (-tags e2e). The previously-skipped RPO=0 invariant is now a hard assertion.
