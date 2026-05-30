---
id: bug-squash-diff-exit-code-ignored
kind: story
stage: drafting
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
