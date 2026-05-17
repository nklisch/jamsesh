---
id: epic-cc-plugin-hooks-retry-queue-and-simple-hooks
kind: story
stage: implementing
tags: [plugin]
parent: epic-cc-plugin-hooks
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Plugin Hooks — Retry Queue + Simple Hooks

## Scope

Build the per-session retry queue, the push-error classifier, the `pre-tool-use` hook (push-gate), and the `session-end` hook (v1 no-op).

## Units delivered

- `cmd/jamsesh/retryqueue/queue.go` + `_test.go` — file-backed JSON FIFO with Load/Save/Enqueue/Drain/Size
- `cmd/jamsesh/pusherr/classify.go` + `_test.go` — error class taxonomy
- `cmd/jamsesh/hooks/pretooluse.go` + `_test.go` — push-gate Bash denial
- `cmd/jamsesh/hooks/sessionend.go` — v1 no-op
- `cmd/jamsesh/main.go` (edit) — register all 6 hook subcommands as a `hook` parent command with subcommands; for unimplemented ones (session-start, user-prompt-submit, post-tool-use, stop), register stubs returning empty JSON

## Acceptance Criteria

- [ ] Retry queue round-trip: enqueue 3 entries; Load returns 3; Drain returns 3 and clears
- [ ] Atomic Save: temp file + rename
- [ ] Classifier: 4xx with `push.scope_violation` → Permanent; 5xx → Transient; network error → Transient
- [ ] pre-tool-use: tool_name=Bash + command=`git push origin main` → deny; command=`git status` → pass
- [ ] pre-tool-use: tool_name=Bash + command=`git config remote.origin.url ...` → deny
- [ ] session-end: returns empty (no-op for v1)
- [ ] `jamsesh hook pre-tool-use` subcommand reads JSON from stdin, writes JSON to stdout

## Notes

- The `cmd/jamsesh/hookio.Run` scaffold (from binary-foundation) handles the JSON IO. Each hook subcommand uses it.
- Retry queue path: `<PluginDataDir>/sessions/<sessionID>/retry-queue.json`. Mode 0600 for the directory's files.
- Sibling story `fetch-push-and-stop-hooks` implements the remaining 4 hooks and depends on this story for retry queue + classifier.
