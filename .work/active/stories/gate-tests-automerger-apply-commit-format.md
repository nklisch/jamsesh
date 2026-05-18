---
id: gate-tests-automerger-apply-commit-format
kind: story
stage: drafting
tags: [testing, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Auto-merger `Apply` clean-merge has no test for trailer ordering / committer identity

## Priority
Medium

## Spec reference
Item: `epic-auto-merger-outcomes-apply`
Acceptance criterion: clean merge sets author=source-author,
committer=auto-merger, trailers (Auto-Merger:true, Source-Commit,
Source-Ref), advances draft, emits merge.succeeded.

## Gap type
missing test for valid partition (trailer ordering + value
verification). Without a test that decodes the commit object and asserts
on the exact trailer key set, the contract is implicit.

## Suggested test
```go
// TestAutoMerger_Apply_CleanMerge_CommitFormat
//   After Apply, decode the new merge commit with go-git.
//   Assert: author.Email == source-author email, committer == "auto-merger@<portalHost>",
//   trailers contain Auto-Merger:true, Source-Commit:<sha>, Source-Ref:<ref> in that order.
//   Verify Resolves-Conflict trailer absent (it's only on resolving commits).
```

## Test location (suggested)
`internal/portal/automerger/outcomes_test.go`
