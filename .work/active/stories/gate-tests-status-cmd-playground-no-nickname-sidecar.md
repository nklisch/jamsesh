---
id: gate-tests-status-cmd-playground-no-nickname-sidecar
kind: story
stage: implementing
tags: [testing, plugin, cli, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-25
updated: 2026-05-25
---

# `status` cmd — playground session with no nickname sidecar not covered

## Priority
High

## Spec reference
Item: `story-status-nickname-empty-playground`
Review notes:
> Backward-compatible: pre-fix sessions without the file render empty
> (today's behaviour); post-fix sessions display the nickname.
Acceptance: "Reproducible across both fresh-create and reload-from-disk
paths."

## Gap type
Backward-compat / boundary case — "today's behaviour" assertion is
undocumented in tests.

## Location
`cmd/jamsesh/sessioncmd/status_test.go` — `TestStatusAction_playgroundSession`
writes nickname via `setupPlaygroundSession` and verifies it renders, but
there is no symmetric `TestStatusAction_playgroundSession_noNicknameSidecar`
covering the backward-compat path (sidecar absent → empty nickname row,
no crash, no stray error).

## Suggested test
```go
func TestStatusAction_playgroundSession_noNicknameSidecar(t *testing.T) {
  // setup per-session token + org_id WITHOUT writing the nickname sidecar
  // (simulates a pre-fix session loaded from disk)
  // ...
  // assert command exits 0 and the playground row renders without the
  // nickname (empty string, no crash, no stray error).
}
```

## Test location (suggested)
`cmd/jamsesh/sessioncmd/status_test.go` — add alongside the existing
playground-session test.

## Impact
The regression that motivated this story (silent blank nickname) could
recur without detection if the readback path ever changed to fatal on
missing sidecar.
