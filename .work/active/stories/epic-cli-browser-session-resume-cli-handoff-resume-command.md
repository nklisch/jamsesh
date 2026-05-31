---
id: epic-cli-browser-session-resume-cli-handoff-resume-command
kind: story
stage: done
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

## Implementation notes

- `ResumeCommand()` + `resumeAction` + `resolveResumeSession()` added to
  `cmd/jamsesh/sessioncmd/resume.go` in the same file as `mintAndOpenResume`.
- Resolver uses `state.CurrentSessionID()` (CLAUDE_SESSION_ID-based) for the
  bare case, falling back to single-session auto-select only when
  CLAUDE_SESSION_ID is unset. Multi-session + unmapped or CC-instance-set but
  unmapped → error citing `jamsesh status`.
- `portalclient.Client{SessionID: sessionID}` construction ensures per-session
  bearer is used for the mint request (both durable and playground paths).
- `portalclient.WireRefresh(pc)` attached for 401-refresh support.
- Mint failure → nonzero exit, `openSilent` never called (no token-free fallback,
  unlike the `--open` adoption path).
- `cmd/jamsesh/main.go`: `ResumeCommand()` registered after `StatusCommand()`.
- Tests in `resume_test.go`: resolver matrix (explicit, bare-CC, bare-single,
  multi-unmapped, CC-set-unmapped), mint failure (nonzero + nothing opened +
  no fallback), bearer verification (per-session token used in mint request).
- `go build ./...`, `go vet ./cmd/jamsesh/...`, `go test ./cmd/jamsesh/sessioncmd/...`
  all pass (8 new tests, full suite green).
