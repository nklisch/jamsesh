---
id: epic-cli-browser-session-resume-cli-handoff-resume-command
kind: story
stage: implementing
tags: [plugin]
parent: epic-cli-browser-session-resume-cli-handoff
depends_on: [epic-cli-browser-session-resume-cli-handoff-mint-open-adopt]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# `jamsesh resume [session-id]` subcommand

Implements **Unit 2** of `epic-cli-browser-session-resume-cli-handoff`. Uses the
`mintAndOpenResume` helper + `OpenSilent` from Unit 1. See the feature body.

## Scope

- `cmd/jamsesh/sessioncmd/resume.go`: `ResumeCommand()` (urfave/cli) — optional
  arg `[session-id]`. Resolution:
  - explicit `<session-id>` → that session;
  - bare → `state.CurrentSessionID()` (write-consistent, `CLAUDE_SESSION_ID`;
    NOT `ResolveSession` — see backlog `cli-resolvesession-env-var-mismatch`);
  - CC-instance env present but unmapped → error citing `jamsesh status`;
  - outside CC context with exactly one local session → resume it;
  - multiple sessions + unmapped → error citing `jamsesh status`.
  Read `org_id` from session state; build `pc` with `SessionID` set; call
  `mintAndOpenResume`. Mint failure → ERROR (nonzero exit), open nothing (NO
  token-free fallback — identity adoption is the command's purpose).
- `cmd/jamsesh/main.go`: register `ResumeCommand()` as a top-level command
  (mirror how `StatusCommand`/`finalize` register).

## Acceptance criteria

- [ ] `jamsesh resume <id>` mints+opens for that session; bare `jamsesh resume`
      resolves the current-instance session via `state.CurrentSessionID`.
- [ ] Multiple sessions + unmapped instance → error citing `jamsesh status`,
      opens nothing, no token printed.
- [ ] Mint failure → nonzero exit, nothing opened.
- [ ] Registered top-level; `go build ./...`, `go vet`, `go test ./cmd/jamsesh/...` pass.

## Notes

References backlog `cli-resolvesession-env-var-mismatch` (root fix of the
resolver inconsistency is out of scope here; use the write-consistent resolver).
