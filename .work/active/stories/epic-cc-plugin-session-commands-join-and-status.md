---
id: epic-cc-plugin-session-commands-join-and-status
kind: story
stage: review
tags: [plugin]
parent: epic-cc-plugin-session-commands
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Session Commands — Join + Status

## Scope

`jamsesh join` and `jamsesh status` subcommands.

## Units delivered

- `cmd/jamsesh/sessioncmd/join.go` + test
- `cmd/jamsesh/sessioncmd/status.go` + test
- `cmd/jamsesh/main.go` (edit) — register both subcommands

## Acceptance Criteria

- [ ] `jamsesh join <session-id>`: pre-auth check; resolves session via REST; clones bare repo; checks out user's ref; writes per-session state; prints summary
- [ ] `jamsesh join <invite-url>`: extracts session-id + invite-id + token; accepts invite first
- [ ] `jamsesh status`: prints text summary by default; `--json` outputs structured JSON
- [ ] Tests use mocked portal (httptest) and either a temp git server or stubbed `runGit` injection

## Notes

- Per-session state path: `${CLAUDE_PLUGIN_DATA}/sessions/<sessionID>/`; create dir if missing.
- `instance_id` from env `CLAUDE_SESSION_ID` if set, else generate a ULID.
- `status` reads `lastSeq` from state and passes it as `?since=` to digest.

## Implementation notes

### Files delivered
- `cmd/jamsesh/sessioncmd/join.go` — `JoinCommand()` + `parseSessionArg`, `findOrgForSession`, `buildCloneURL`, `writeSessionState` helpers
- `cmd/jamsesh/sessioncmd/join_test.go` — 6 tests (parse variants, happy join, invite URL join, missing arg)
- `cmd/jamsesh/sessioncmd/status.go` — `StatusCommand()` + `readSessionState`, `truncate` helpers
- `cmd/jamsesh/sessioncmd/status_test.go` — 4 tests (text output, JSON output, comment filtering, readSessionState)
- `cmd/jamsesh/main.go` — added `sessioncmd.JoinCommand()` and `sessioncmd.StatusCommand()` to the Commands slice

### Design decisions
- `runGit` and `runGitOutput` are package-level `var` functions so fork tests (sibling story) can stub them; join tests reassign them in-test.
- `resolveSession()` (from sibling's `session.go`) is reused for the session ID lookup; `readSessionState()` is a new helper in `status.go` that reads `org_id` and `ref` from the state dir.
- `writeSessionState` persists `ref`, `instance_id`, `last_seen_seq`, `account_id`, and `org_id` (org_id added so `status` avoids re-scanning orgs on every call).
- Test mock `/api/me` handlers write raw JSON to avoid `openapi_types.Email` marshal validation failure on zero-value struct fields.
- `parseSessionArg` supports bare session ID, `orgID/sessionID`, and full invite URL with `?org=&session=&invite=&token=` query params.

### Acceptance criteria status
- [x] `jamsesh join <session-id>`: pre-auth check; resolves session via REST; clones bare repo; checks out user's ref; writes per-session state; prints summary
- [x] `jamsesh join <invite-url>`: extracts session-id + invite-id + token; accepts invite first
- [x] `jamsesh status`: prints text summary by default; `--json` outputs structured JSON
- [x] Tests use mocked portal (httptest) and stubbed `runGit` injection
