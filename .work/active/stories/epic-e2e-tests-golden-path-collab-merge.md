---
id: epic-e2e-tests-golden-path-collab-merge
kind: story
stage: implementing
tags: [e2e-test, testing]
parent: epic-e2e-tests-golden-path
depends_on: [epic-e2e-tests-golden-path-session-lifecycle]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Golden — Collaboration + auto-merger + MCP tool use

## Scope

Two Go specs that together prove the auto-merger converges
non-conflicting work into `draft`, that agent MCP tool calls work
(post_comment, resolve_comment, fork), and that addressed comments
reach the addressee's next-turn digest.

- `tests/e2e/golden/auto_merge_test.go` — Agent A and B push
  non-conflicting changes; auto-merger advances `draft`; both
  `merge.succeeded` events fire
- `tests/e2e/golden/fork_and_comment_test.go` — Agent A calls `fork`
  via MCP, posts a comment via MCP addressed to Agent B; Agent B's
  next `user-prompt-submit` hook surfaces the comment in
  `additionalContext`

## Auto-merge spec invariant

After Agent A pushes commit X on a sync ref and Agent B pushes commit Y
on a different sync ref (no file overlap), `git fetch && git log draft`
shows both commits reachable, AND two `merge.succeeded` events appear
in the WebSocket event stream.

## Fork-and-comment spec invariant

After Agent A calls `fork` via MCP from a draft commit and posts a
comment addressed to `@agent-b`, Agent B's `user-prompt-submit` hook
returns `additionalContext` containing the comment text.

## Files to create / modify

- `tests/e2e/golden/auto_merge_test.go` — the auto-merge spec
- `tests/e2e/golden/fork_and_comment_test.go` — the fork+comment spec
- `tests/e2e/fixtures/mcpclient/mcpclient.go` (NEW) — small helper
  that exposes typed wrappers for the four MCP tools (`post_comment`,
  `resolve_comment`, `fork`, `query_session_state`). Uses the official
  `github.com/modelcontextprotocol/go-sdk` client to connect via
  streamable-http. Bearer-token auth.

## Acceptance criteria

- [ ] Auto-merge spec green: non-conflicting pushes both reach
      `draft` without manual intervention; `merge.succeeded` events
      fire for both source commits
- [ ] Fork spec green: `fork` MCP call returns a new ref
      `jam/<sid>/<user>/fork-<sha7>`; `ref.forked` event fires on the
      WS stream
- [ ] Comment-addressing spec green: Agent A's `post_comment` with
      `addressed_to: "@agent-b"` flows into Agent B's
      `user-prompt-submit` hook `additionalContext`
- [ ] All assertions on user-visible outcomes (HTTP responses, git
      log output, WS event payloads, hook stdout) — no
      mock-invocation assertions

## Notes for the implementer

- MCP tools live in `internal/portal/mcpendpoint/tools.go`. Wire
  format documented in `docs/openapi.yaml` and `docs/PROTOCOL.md`.
- The auto-merger runs in-process in the portal binary via the
  `automerger.Worker` started in `cmd/portal/main.go`. Its event
  emission is what the test asserts on (via WS subscription).
- The MCP SDK has a client side: `mcp.NewClient` +
  `mcp.NewStreamableHTTPTransport`. The bearer token goes in the
  Authorization header via the transport's `RequestEditorFn`.
- For comment addressing semantics see
  `docs/PROTOCOL.md > Comment schema`.
