---
id: epic-cc-plugin-session-commands-join-and-status
kind: story
stage: implementing
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
