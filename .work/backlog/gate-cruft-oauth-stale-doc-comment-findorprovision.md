---
id: gate-cruft-oauth-stale-doc-comment-findorprovision
kind: story
stage: drafting
tags: [cleanup]
parent: null
depends_on: []
release_binding: null
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# Slightly stale doc-comment — references removed `FindOrProvision` instead of current `FindOrProvisionAt`

## Confidence
Low

## Category
stale comment

## Location
`internal/portal/auth/oauth.go:176-177`

## Evidence
```go
// Map the provider Identity to the shared auth.Identity type used by
// FindOrProvision.
id := Identity{ ... }
acc, _, err := FindOrProvisionAt(ctx, h.store, id, h.clock.Now())
```

## Removal
Update the comment to say `FindOrProvisionAt` (the actual callee). `FindOrProvision` is the deadcode-flagged unreachable function at `internal/portal/auth/provision.go:42` (not in this bundle) — likely scheduled for removal in a separate pass, but the doc-comment drift was introduced here.
