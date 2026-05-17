---
id: epic-cc-plugin-hooks-retry-queue-and-simple-hooks
kind: story
stage: review
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

## Implementation notes

### Packages delivered

- `cmd/jamsesh/retryqueue/queue.go` — file-backed JSON FIFO. `Queue.Path()` builds the session-scoped path. `Save()` creates the directory tree on first write, uses temp+rename for atomicity, chmod 0600 before rename. `Drain()` is a single operation: Load → Save([]) → return.
- `cmd/jamsesh/retryqueue/queue_test.go` — 6 tests: emptyLoad, roundTrip (enqueue-3 + verify FIFO order), drain (clears after), size, atomicSave (no temp leakage), fileMode (0600).
- `cmd/jamsesh/pusherr/classify.go` — `Classify(httpStatus int, body []byte) Result`. Status 0 → Transient; 2xx → OK; 5xx → Transient (body parsed best-effort); 4xx → Permanent with body parsed. All `push.*` codes and `auth.{invalid_token,insufficient_permission,expired_token}` are Permanent; generic 4xx also Permanent.
- `cmd/jamsesh/pusherr/classify_test.go` — 9 tests covering all class branches, auth codes, details extraction, malformed body.
- `cmd/jamsesh/hooks/io.go` — `WithIO` / `stdinOf` / `stdoutOf` context helpers for test injection.
- `cmd/jamsesh/hooks/pretooluse.go` — `PreToolUse` cli action; denies `git push` (any form, leading-whitespace-trimmed) and `git\s+config\s+remote\.` regex; passes everything else.
- `cmd/jamsesh/hooks/pretooluse_test.go` — 11 tests covering deny/pass cases.
- `cmd/jamsesh/hooks/sessionend.go` — v1 no-op; reads SessionEnd payload, returns `{}`.
- `cmd/jamsesh/hooks/stubs.go` — `SessionStart`, `UserPromptSubmit`, `PostToolUse`, `Stop` stubs returning `{}`.
- `cmd/jamsesh/main.go` (edited) — `hookCommand()` registers all 6 subcommands under `jamsesh hook`.

### Collateral fix

The sibling `sessioncmd` package (in-progress untracked work) had a `runGit` redeclaration that caused the main binary to fail to build (linter wired it into main.go). The linter already cleaned up `session.go`; `fork.go` was also already corrected. The `sessioncmd` test suite still has an incomplete `buildCLIApp` helper — that is left for the sibling story.

### Test results

`go test ./cmd/jamsesh/retryqueue/... ./cmd/jamsesh/pusherr/... ./cmd/jamsesh/hooks/...` — 26/26 PASS.
`go build ./cmd/jamsesh/` — clean.
