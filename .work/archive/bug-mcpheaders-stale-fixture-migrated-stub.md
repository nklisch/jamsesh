---
id: bug-mcpheaders-stale-fixture-migrated-stub
kind: story
stage: done
tags: [portal, plugin, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
---

# Bug: mcpheaders tests fail with stale fixture (MIGRATED_TO_PER_SESSION)

## Symptoms

`go test ./cmd/jamsesh/mcpheaders/...` fails:

```
--- FAIL: TestMcpHeaders_tokenPresent (0.31s)
    mcpheaders_test.go:81: Authorization = "Bearer MIGRATED_TO_PER_SESSION", want "Bearer my-test-token"
--- FAIL: TestMcpHeaders_tokenAndSession
    ...
```

## Root cause

The `mcpheaders` tests write a token to the legacy global token file path and
expect the header builder to read it back. But the plugin-skills bearer-storage
story (`story-epic-ephemeral-playground-plugin-skills-bearer-storage`) introduced
`state.MigrateToPerSessionTokens()`, which replaces the legacy token file with a
`MIGRATED_TO_PER_SESSION` stub. The test state/setup triggers the migration
path, so the header builder reads the stub instead of the test token.

## Fix

Update the `mcpheaders` tests to write tokens to the per-session token path
(rather than the legacy global path), matching the post-migration behavior.
Alternatively, have the test use a temporary directory that has NOT been
migrated (no `MIGRATED_TO_PER_SESSION` stub).

## Discovered during

Implementation of `story-epic-ephemeral-playground-session-lifecycle-destruction`.
Pre-existing failure confirmed by `git stash` / `git stash pop` round-trip.

## Implementation notes

**Approach chosen: already fixed by prior work — no test changes required.**

Investigation revealed that `story-state-readtoken-sweep-step-2-callsites`
(commit `80792c2`) had already resolved the root cause before this story was
scoped. That commit changed `mcpheaders.go` from calling `state.ReadToken()`
directly to calling `state.ReadCurrentBearer("")`, and added the per-session
bearer read path (`state.ReadSessionToken(sessID)`) before the legacy fallback.

The interaction that makes the tests pass cleanly under migration:

- `TestMcpHeaders_tokenPresent` — no `sessions/` directory → `ListSessions()`
  returns empty → migration is a no-op → legacy token file remains intact.
- `TestMcpHeaders_tokenAndSession` — `sessions/<jamSessionID>/` exists from the
  `instance_id` write → migration fans the legacy token out to
  `sessions/<jamSessionID>/token` → `mcpheaders` reads the per-session token
  directly via `ReadSessionToken(sessID)`, bypassing the stub entirely.

`go test ./cmd/jamsesh/mcpheaders/... -count=1` passes all 4 tests.
`go test ./cmd/jamsesh/... -count=1` passes the full cmd suite with no regressions.

## Review (2026-05-24)

**Verdict**: Approve

**Notes**:

Agent verified the bug was already resolved by prior commit `80792c2`
(`story-state-readtoken-sweep-step-2-callsites`) which rewired
`mcpheaders.go` to use `state.ReadCurrentBearer("")` — that helper
reads per-session bearer before falling back to legacy. The two
failing tests now pass naturally:
- `TestMcpHeaders_tokenPresent`: no sessions/ dir → migration is no-op → legacy token preserved
- `TestMcpHeaders_tokenAndSession`: instance_id write creates sessions/<id>/ → migration fans token there → mcpheaders reads per-session directly

`go test ./cmd/jamsesh/mcpheaders/... -count=1` passes (1.244s).

This is the honest discovery shape: bug was real when filed, fixed by
unrelated cleanup work, story closes without code change. No silent
regression risk since the contract is now locked in by the existing
passing tests.

Advanced `stage: review → done`.
