---
id: gate-tests-cli-playground-push-failure-recovery
kind: story
stage: implementing
tags: [testing, plugin, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# CLI `--playground` push-failure: session-stays-live-with-null-base_sha contract is not tested

## Priority
High

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-cli-playground-flag`

Acceptance criterion: Implementation notes: "Push-failure behavior matches the locked decision: the session stays live with `base_sha: null` and the user gets a retry command pointing at the same remote URL (`wrapPlaygroundPushError`)."

## Gap type
missing test for error case

## Suggested test
```go
func TestPlaygroundAction_pushFailureLeavesSessionLiveWithRetry(t *testing.T) {
    // 1. Portal fake: 201 from POST /api/playground/sessions, push-transport error.
    // 2. Call newPlaygroundAction.
    // 3. Assert wrapPlaygroundPushError (not generic error).
    // 4. Assert printed retry command + remote URL.
    // 5. Assert per-session state files (token, org_id, ref) still written.
}
```
`TestNewAction_pushFailureLeavesSessionLive` covers the durable path; the
playground path has no equivalent.

## Test location (suggested)
`cmd/jamsesh/sessioncmd/new_test.go`
