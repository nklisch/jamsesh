---
id: epic-cc-plugin-hooks-fetch-push-and-stop-hooks
kind: story
stage: implementing
tags: [plugin]
parent: epic-cc-plugin-hooks
depends_on: [epic-cc-plugin-hooks-retry-queue-and-simple-hooks]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Plugin Hooks — Fetch / Push / Stop

## Scope

Implement the 4 substantive hooks: session-start, user-prompt-submit, post-tool-use, stop. Uses retry queue + classifier from the prior story.

## Units delivered

- `cmd/jamsesh/hooks/sessionstart.go` + test
- `cmd/jamsesh/hooks/userpromptsubmit.go` + test
- `cmd/jamsesh/hooks/posttooluse.go` + test
- `cmd/jamsesh/hooks/stop.go` + test
- `cmd/jamsesh/main.go` (edit) — replace the stub subcommands with real handlers

## Acceptance Criteria

- [ ] session-start: calls `/api/sessions/<id>`, `/refs`, `/comments?addressed_to=...`; assembles additionalContext text
- [ ] user-prompt-submit: runs `git fetch`, drains retry queue, calls `/digest?since=<lastSeq>`, returns combined additionalContext, advances lastSeq in local state
- [ ] post-tool-use: detects successful `git commit` (tool_name=Bash, exit_code=0, command starts with "git commit"); pushes with 3-retry policy; transient-all-fail enqueues; permanent fail returns additionalContext with error details
- [ ] stop: dirty tree → auto-commit + push; queue > 10 → refuse with stderr error; POST turn.ended
- [ ] All tests use mocked portal (httptest) and mocked git (os/exec stub or temp repo)

## Notes

- The portal client from binary-foundation (`cmd/jamsesh/portalclient`) handles auth + 401-refresh. Use it directly for all portal calls.
- `lastSeq` per session in `<PluginDataDir>/sessions/<sid>/last_seen_seq` (write via state.Write).
- The hooks call `git` via `os/exec`. For tests, inject a `runGit func(...) (out, err)` function for stubbing.
- `additionalContext` is a free-form text block; format follows PROTOCOL.md's digest section conventions.
