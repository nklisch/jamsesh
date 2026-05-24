---
id: gate-tests-reserved-slug-conflict-main-exit-1
kind: story
stage: implementing
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Reserved-slug-conflict `cmd/portal/main.go` exit-1 path is not tested

## Priority
High

## Spec reference
Item: `feature-epic-ephemeral-playground-reserved-org`

Acceptance criterion: Unit 4 AC: "Pre-existing unprotected org with slug `playground`: returns `ErrReservedSlugConflict`, main exits 1 with a clear error." SELF_HOST.md documents this behavior.

## Gap type
missing test for e2e-seam (function tested in isolation; main wiring untested)

## Suggested test
```go
func TestMain_PlaygroundEnabledWithUnprotectedSlugCollision_Exits1(t *testing.T) {
    // Start portal binary subprocess: PlaygroundEnabled=true,
    // seeded with an unprotected org slug='playground'.
    // Assert exit code 1 + stderr contains "reserved slug" + remediation hint.
}
```
`TestProvisionReservedOrg_UnprotectedSlugConflict` validates the function
return but not the `os.Exit(1)` operators see.

## Test location (suggested)
`cmd/portal/main_test.go` (new file)
