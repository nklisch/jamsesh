---
id: gate-security-mcp-fork-ref-name-validation
kind: story
stage: done
tags: [security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Ref-name traversal in `fork` MCP tool can clobber session base/draft refs

## Severity
High

## Domain
Input Validation & Injection

## Location
`internal/portal/mcpendpoint/tools.go:179-207`

## Evidence
```go
branchSuffix := "fork-" + sha7
if in.TargetRef != nil && *in.TargetRef != "" {
    branchSuffix = strings.TrimPrefix(*in.TargetRef, "refs/heads/")
}
refName := plumbing.NewBranchReferenceName(
    fmt.Sprintf("jam/%s/%s/%s", in.SessionID, info.UserID, branchSuffix),
)
...
if err := repo.Storer.SetReference(ref); err != nil {
```

`branchSuffix` is never validated, and go-git's `SetReference` does not
invoke `ReferenceName.Validate()`. A caller supplying
`target_ref: "../../base"` yields
`refs/heads/jam/<sess>/<acct>/../../base`, which the filesystem storer
normalises to `refs/heads/jam/<sess>/base` — clobbering the session's
base ref controlled by `receive_pack.go:206`. The same trick targets the
draft ref, the auto-merger's intake ref, or any other ref under
`refs/heads`.

## Remediation direction
After constructing `refName`, call `refName.Validate()` and reject
non-nil errors with a 400. Additionally enforce that the suffix matches
`^[A-Za-z0-9_.-]+$` (no slashes, no dots-at-start) before composing the
ref so even minor go-git rule gaps are blocked.

## Implementation notes

**Validation regex:** `^[A-Za-z0-9_][A-Za-z0-9_.-]*$`
- Must start with alphanumeric or underscore (rejects leading `-` and `.`).
- Allows alphanumerics, underscores, hyphens, and dots thereafter.
- Slashes are implicitly rejected (not in character class).
- `..` is rejected by an explicit `strings.Contains(suffix, "..")` check
  applied before the regex, blocking git traversal sequences even if a
  future regex edit missed them.

**Where called:** `internal/portal/mcpendpoint/tools.go` — `validateForkTargetRef`
is invoked in the `fork` handler immediately after `strings.TrimPrefix` strips
`refs/heads/`, and before `plumbing.NewBranchReferenceName` composes the full path.

**Defence-in-depth:** After composing `refName`, `refName.Validate()` is also
called. Any suffix that passes the regex but still violates go-git's own rules
is caught here with a second error path.

**Error type and code returned:** Both validation paths return a plain `error`
wrapped with `fmt.Errorf("fork: %w", err)`. The MCP SDK maps any non-nil error
from a tool handler to a `tool_error` response (`isError: true` in the JSON
content envelope), consistent with the existing `TestMCPEndpoint_Fork_BadCommit`
test pattern.

**Happy-path preserved:** Default suffix `fork-<sha7>` (e.g. `fork-abc1234`)
starts with `f` and contains only alphanumerics and a hyphen — passes
`validateForkTargetRef`. User-supplied names like `feature-x` or `my_branch.1`
also pass. Existing tests continue green.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: High-severity ref-traversal closed. validateForkTargetRef rejects empty, '..' substrings, and any suffix not matching ^[A-Za-z0-9_][A-Za-z0-9_.-]*$. Called immediately after strings.TrimPrefix and before plumbing.NewBranchReferenceName. refName.Validate() invoked as defence-in-depth. Happy path (fork-<sha7>) still passes. Existing tests green. Nit: this commit also captured the k8s discoverer deletions (PostToolUse hook quirk during parallel wave) — the deletions were sanctioned by gate-cruft-router-kube-discovery-wired-or-deleted and are not a defect of this story.
