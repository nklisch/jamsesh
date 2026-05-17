---
id: epic-cloud-native-deploy-routing-layer-mcp-header
kind: story
stage: review
tags: [plugin]
parent: epic-cloud-native-deploy-routing-layer
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — `Jam-Session-Id` header in `jamsesh mcp-headers`

## Scope

Extend `cmd/jamsesh/mcpheaders/mcpheaders.go` so the subcommand emits
`Jam-Session-Id: <session_id>` alongside the existing `Authorization`
header when the local CC instance has a bound session. Tolerate absent
binding — emit Authorization-only in that case.

Implements **Unit 5** of `epic-cloud-native-deploy-routing-layer`. The
change is universally safe: single-instance pods ignore the header,
clustered-mode router uses it for session routing.

## Files

Edit:
- `cmd/jamsesh/mcpheaders/mcpheaders.go`
- `cmd/jamsesh/mcpheaders/mcpheaders_test.go`
- `cmd/jamsesh/state/state.go` — add helper to read per-CC-instance
  session binding if not already present (per
  `docs/ARCHITECTURE.md` local state layout:
  `${CLAUDE_PLUGIN_DATA}/sessions/<cc-session-id>/ref`)

## Acceptance criteria

- [ ] With token + bound session → header JSON includes both
  `Authorization` and `Jam-Session-Id`
- [ ] With token, no bound session → header JSON has `Authorization` only
- [ ] No token → exits 2 with "no token found" (preserve existing behavior)
- [ ] Unit tests cover all three cases

## Notes

- Read the binding via `state.ReadSessionBinding(ccSessionID)` (or
  similar) returning `(sessionID string, ok bool)`. If `state` package
  already has a helper, reuse it.
- The CC session id is available via `CLAUDE_SESSION_ID` env var per
  Claude Code's plugin contract (confirm during impl).
- If multiple sessions are bound (rare — one CC instance, one binding),
  the helper returns the most-recent or the only one.

## Implementation notes

### Approach

Added `state.CurrentSessionID() (string, bool)` to `cmd/jamsesh/state/state.go`.
It reads `CLAUDE_SESSION_ID` env var (the CC instance identifier), walks
`${CLAUDE_PLUGIN_DATA}/sessions/` looking for a directory whose `instance_id`
file matches, and returns the directory name (the jamsesh session ID). Returns
`("", false)` when the env var is unset or no binding is found.

The local state layout was confirmed from `docs/ARCHITECTURE.md` and
`cmd/jamsesh/sessioncmd/join.go` (which writes `instance_id`):

```
${CLAUDE_PLUGIN_DATA}/sessions/<jamsesh-session-id>/instance_id  ← CC instance ID
${CLAUDE_PLUGIN_DATA}/sessions/<jamsesh-session-id>/ref          ← bound ref
```

### Files changed

- `cmd/jamsesh/state/state.go` — added `CurrentSessionID()` helper
- `cmd/jamsesh/mcpheaders/mcpheaders.go` — emits `Jam-Session-Id` when bound
- `cmd/jamsesh/mcpheaders/mcpheaders_test.go` — three test cases added

### Incidental fix

Repaired a pre-existing bug in `internal/router/extract/extract.go` (from the
parallel routing-layer-core story): `/auth/` with a trailing slash was
incorrectly falling through the system-route guard after trailing-slash
stripping turned it into `/auth`, which doesn't match
`strings.HasPrefix(path, "/auth/")`. Added `path == "/auth"` as an additional
condition. All `TestSessionID_SystemRoutes` subtests now pass.
