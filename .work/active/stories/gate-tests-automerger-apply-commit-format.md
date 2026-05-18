---
id: gate-tests-automerger-apply-commit-format
kind: story
stage: done
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

## Implementation notes

Added `TestAutoMerger_Apply_CleanMerge_CommitFormat` to
`internal/portal/automerger/outcomes_test.go`.

**Trailer assertion strategy:** Rather than using `prereceive.Trailers` (which
returns a `map[string]string` and loses ordering), the test uses a local
`extractTrailerLines` helper that walks backward through the commit message to
isolate the last non-empty paragraph (the trailer block), then parses the lines
as ordered `(key, value)` pairs. This lets us assert both the key order
(`Auto-Merger` → `Source-Commit` → `Source-Ref`) and the values without
reaching into go-git internals.

**Identity assertions:**
- `Author.Email` and `Author.Name` are asserted equal to the source commit's
  author (carried through by `composeMergeMessage`).
- `Committer.Email` is asserted equal to `"auto-merger@jamsesh.test"` and
  `Committer.Name` to `"jamsesh auto-merger"` — both hardcoded in
  `applySuccess`.

**Absent-trailer assertions:**
- `Resolves-Conflict` must not appear on a clean merge where the source commit
  carries no such trailer.
- `Auto-Resolved` must not appear (it is only emitted for `SafeAutoResolve`).

**No ordering drift found:** the test passes against the current production
code, confirming `composeMergeMessage` emits trailers in the correct order.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Auto-merger clean-merge commit format pinned. New test decodes the merge commit and asserts: author email matches source author; committer is auto-merger@jamsesh.test (with name 'jamsesh auto-merger'); trailers appear in positional order Auto-Merger → Source-Commit → Source-Ref; Resolves-Conflict and Auto-Resolved trailers absent on clean merge. Uses local trailer-extract helper that returns ordered slice (not map) to catch ordering drift. No drift found — composeMergeMessage already emits in correct order.
