---
id: gate-tests-status-cmd-playground-no-nickname-sidecar
kind: story
stage: review
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

## Implementation notes

Added `TestStatusAction_playgroundSession_noNicknameSidecar` to
`cmd/jamsesh/sessioncmd/status_test.go` immediately after the existing
`TestStatusAction_playgroundSession`.

**Design-flaw check:** Reviewed `readNickname` (status.go:262–272) — it
already handles missing sidecar gracefully: any `os.ReadFile` error (including
`fs.ErrNotExist`) returns `""` with no stderr output. No bug found; no escape
hatch needed.

**Test handle:** `setupPlaygroundSession` skips the sidecar write when
`nickname == ""` (line 55–58), so passing an empty string simulates the
pre-fix on-disk state without extra helper code.

**Assertions (tighter than the minimum):**
- exit 0 (no error returned)
- stdout contains "Playground sessions" section header
- stdout contains the session ID
- stdout contains "Ends in:" duration
- stderr is completely empty (no stray warnings — the regression would emit
  an error mentioning "nickname" if `readNickname` ever started erroring
  rather than silently returning "")

Both variants pass; `go vet` is clean.
