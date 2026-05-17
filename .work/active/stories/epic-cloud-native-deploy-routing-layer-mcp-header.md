---
id: epic-cloud-native-deploy-routing-layer-mcp-header
kind: story
stage: implementing
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
