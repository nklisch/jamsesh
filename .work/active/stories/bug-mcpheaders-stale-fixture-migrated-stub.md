---
id: bug-mcpheaders-stale-fixture-migrated-stub
kind: story
stage: implementing
tags: [portal, plugin, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
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
