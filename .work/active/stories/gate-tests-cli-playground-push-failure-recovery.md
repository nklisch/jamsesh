---
id: gate-tests-cli-playground-push-failure-recovery
kind: story
stage: review
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

## Implementation notes

Added `TestPlaygroundAction_pushFailureLeavesSessionLiveWithRetry` to
`cmd/jamsesh/sessioncmd/new_test.go` (after the existing playground tests,
before `TestPlaygroundAction_pushUsesBearerNotOAuthToken`).

**Approach:** Used the established `stubGitForNew(t, pushErr)` helper to inject
a simulated push failure, `setupPlaygroundEnv` for the unauthenticated playground
env, and an `httptest.NewServer` fake portal that returns 201 with a
`PlaygroundSessionCreated` response. A catch-all handler records any
abandon/delete calls.

**What the test asserts:**
1. Error carries the `wrapPlaygroundPushError` sentinel phrases: "playground
   session", "base_sha: null", the session ID, `git push`, the remote URL
   (`<srv.URL>/git/<id>.git`), and the refspec (`refs/heads/jam/<id>/base`).
2. No abandon call is made — session stays live per locked decision.
3. The per-session `token` file IS written before the push (written by
   `state.WriteSessionToken` at line 405 of `new.go`, before `pushBaseRefWithBearer`).
4. `org_id` and `ref` files are NOT present — they are written by
   `writePlaygroundSessionState`, which is only reached after a successful push.
   The test asserts their absence explicitly to pin this behaviour and catch
   any accidental reordering.

**No production bugs found.** The spec language "token, org_id, ref are still
written" refers to the durable-path equivalent; for the playground path, only
`token` is written before the push (by design — `org_id`/`ref` are written
post-push by `writePlaygroundSessionState`). The test documents this honestly.
