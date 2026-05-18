---
id: gate-security-finalize-target-branch-dotdot
kind: story
stage: done
tags: [security, portal, finalize]
parent: null
depends_on: [gate-security-finalize-script-shell-escape]
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# `ValidateTargetBranch` passes `..`-containing paths

## Severity
Medium (security gap, not directly exploitable via shell injection, but
allows creation of git refs with path-traversal segments that git itself
would reject, and is inconsistent with the stated validation contract).

## Discovered by
`TestPatchFinalizeLock_RejectsMaliciousTargetBranch/dotdot_escape` in
`gate-tests-finalize-shell-escape` — the test case `"../escape"` expected
a 400 but received 200.

## Root cause
`escape.go:reTargetBranch` is `^[A-Za-z0-9._/][A-Za-z0-9._/-]*$`.
This regex allows `.` and `/` to appear in any combination, so `..`,
`../escape`, `main/../evil`, and `../../etc/passwd` all pass.

Git itself rejects ref names containing `..` (see
`git check-ref-format` rules: "They cannot have two consecutive dots").
The validator should reject any branch name containing `..` as a component
to stay in sync with what git will accept and to prevent confusion.

## Fix
Add a check in `ValidateTargetBranch` that rejects strings containing `..`:

```go
if strings.Contains(branch, "..") {
    return false
}
```

Or tighten the regex so that `..` can never appear. Preferred: explicit
`strings.Contains` guard because it is self-documenting.

## Test
`TestPatchFinalizeLock_RejectsMaliciousTargetBranch` includes `dotdot_escape`
("../escape") and `dotdot_middle` ("main/../evil") cases — both pass with
the fix applied.

## Implementation notes

Added `strings.Contains(branch, "..")` guard to `ValidateTargetBranch` in
`internal/portal/finalize/escape.go` ahead of the regex check. The two
dotdot test cases that were commented out in the companion test story
are now active and green.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: One-line guard added to `ValidateTargetBranch`. `..` anywhere in
the branch suffix now rejects. Test coverage active.
