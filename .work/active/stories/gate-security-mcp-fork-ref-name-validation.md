---
id: gate-security-mcp-fork-ref-name-validation
kind: story
stage: implementing
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
