---
id: epic-cc-plugin-hooks-fetch-push-and-stop-hooks
kind: story
stage: done
tags: [plugin]
parent: epic-cc-plugin-hooks
depends_on: [epic-cc-plugin-hooks-retry-queue-and-simple-hooks]
release_binding: v0.1.0
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
- `cmd/jamsesh/hooks/stubs.go` (edit) — removed all 4 stubs
- `cmd/jamsesh/main.go` (edit) — updated hook command descriptions

## Acceptance Criteria

- [x] session-start: calls `/api/sessions/<id>`, `/refs`, `/comments?addressed_to=...`; assembles additionalContext text
- [x] user-prompt-submit: runs `git fetch`, drains retry queue, calls `/digest?since=<lastSeq>`, returns combined additionalContext, advances lastSeq in local state
- [x] post-tool-use: detects successful `git commit` (tool_name=Bash, exit_code=0, command starts with "git commit"); pushes with 3-retry policy; transient-all-fail enqueues; permanent fail returns additionalContext with error details
- [x] stop: dirty tree → auto-commit + push; queue > 10 → refuse with stderr error; POST turn.ended (v1: skipped, endpoint not in spec yet)
- [x] All tests use mocked portal (httptest) and mocked git (HookRunGit package var)

## Notes

- The portal client from binary-foundation (`cmd/jamsesh/portalclient`) handles auth + 401-refresh. Used directly for all portal calls.
- `lastSeq` per session in `<PluginDataDir>/sessions/<sid>/last_seen_seq` (write via state.Write).
- The hooks call `git` via `os/exec`. For tests, `HookRunGit` (exported package var) is replaced with a stub.
- `additionalContext` is a free-form text block formatted following the feature spec.
- `extractJSONBody` in userpromptsubmit.go extracts the JSON error envelope from git smart-http stderr, which prefixes it with "error: <N> <phrase>\n".
- POST `turn.ended` is a v1 skip — the endpoint does not yet exist in the API spec.

## Implementation notes

### session-start

- `resolveHookSession()` in `sessionstart.go` mirrors `sessioncmd.resolveSession` locally to avoid circular imports.
- Reads `org_id`, `ref`, `account_id` from per-session state files.
- Calls `/api/me`, `/api/orgs/<orgID>/sessions/<sessionID>`, `/api/orgs/<orgID>/sessions/<sessionID>/refs`, `/api/orgs/<orgID>/sessions/<sessionID>/comments?addressed_to=@<accountID>&resolved=false`.
- Formats additionalContext with sections: session header, Your refs, Peer activity, Unresolved comments.
- If no session is mapped → returns empty `{}`.

### user-prompt-submit

- Runs `git fetch session-remote`; failure is non-fatal (warning in context).
- Drains retry queue; transient re-queued, permanent dropped with stderr log.
- Calls `/api/orgs/<orgID>/sessions/<sessionID>/digest?since=<lastSeq>`.
- Writes updated `next_cursor` to `last_seen_seq` state file.
- `HookRunGit` is the injectable git runner shared by all 4 hook files.
- `pushCommitWithRetry()` contains the 3-attempt exponential backoff logic.
- `extractHTTPStatus()` parses "error: <N> ..." from git smart-http stderr.
- `extractJSONBody()` extracts the JSON object from multi-line stderr.

### post-tool-use

- Filters to: tool_name == "Bash", command starts with "git commit", exit_code == 0.
- Gets current ref via `git rev-parse --abbrev-ref HEAD`.
- Calls `pushCommitWithRetry(ref, 3)`.
- Transient all-fail: enqueues via `retryqueue.Queue`, returns warning in additionalContext.
- Permanent: returns additionalContext with error code + message + details.

### stop

- `git status --porcelain` → dirty check.
- Dirty: `git add -A` + `git commit -m "WIP [jamsesh auto-commit at turn end]" --trailer "Jam-Auto-Commit: true"`.
- Final push with retry.
- Transient push failure → enqueue.
- Queue > 10 → `os.Exit(1)` with stderr message "session is wedged".
- POST turn.ended → v1 skip (documented in code).

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: resolveHookSession inlined to break import cycle is reasonable. extractJSONBody handles git smart-http's 'error: N phrase' stderr prefix cleanly. v1 turn.ended skip documented. 38 tests passing in hooks package.
