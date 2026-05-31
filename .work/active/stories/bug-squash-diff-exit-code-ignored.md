---
id: bug-squash-diff-exit-code-ignored
kind: story
stage: done
tags: [bug, portal, error-handling]
parent: epic-bug-squash-automerger-correctness
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: low
bug_domain: error-handling
bug_location: internal/portal/automerger/heuristics.go:228
---

# diff subprocess exit code fully ignored — exit-2 (real error) parsed as "no differences"

**Location**: `internal/portal/automerger/heuristics.go:228` · **Severity**: low · **Pattern**: git/CLI subprocess exit code not classified

`diff` returns 0 (identical), 1 (differences — expected here), or 2 (trouble: missing binary, unreadable file). `_ = cmd.Run()` discards the error entirely, so an exit-2 failure yields empty/garbage output that is then parsed as a valid diff and fed to the add-only auto-resolve heuristic with wrong data. The heuristic's conservative fallback bounds the blast radius, but a genuine tool failure is invisible. Fix: inspect the `*exec.ExitError` — accept codes 0 and 1, return the error for code 2 or any non-ExitError (e.g. `diff` not on PATH).

```go
cmd := exec.Command("diff", "-u", baseFile, otherFile)
_ = cmd.Run() // diff exits 1 when there are differences — expected (but exit 2 is also swallowed)
return parseUnifiedDiffAddOnly(out.Bytes())
```

## Implementation notes

Extracted two helpers into `heuristics.go`:
- `classifyDiffErr(err error) error` — pure classifier: nil/exit-0/exit-1 → nil;
  exit≥2 → error; non-`*exec.ExitError` → error. No subprocess involvement.
- `runDiff(baseFile, otherFile string) ([]byte, error)` — runs `diff -u` and
  calls `classifyDiffErr` on the error. Returns stdout bytes on success.

`diffAddOnly` now calls `runDiff` instead of `_ = cmd.Run()`, and propagates
the returned error to callers. `isNonOverlappingAddition` already propagated
`diffAddOnly` errors (returns `nil, false`) so no behaviour change there.

Tests added in `diff_exit_code_test.go` (package `automerger`, internal):
- `TestClassifyDiffErr_Exit0_ReturnsNil` — exit 0 is success.
- `TestClassifyDiffErr_Exit1_ReturnsNil` — exit 1 (differences) is success.
- `TestClassifyDiffErr_Exit2_ReturnsError` — exit 2 is a real error.
- `TestClassifyDiffErr_Exit127_ReturnsError` — any exit ≥ 2 is an error.
- `TestClassifyDiffErr_NonExitError_ReturnsError` — non-ExitError (missing binary) is an error.
- `TestDiffAddOnly_HappyPath_IdenticalContent` — identical content, zero hunks.
- `TestDiffAddOnly_HappyPath_PureAddition` — pure addition, hunks returned correctly.
