---
id: epic-e2e-tests-infrastructure-ccdriver
kind: story
stage: done
tags: [e2e-test, testing]
parent: epic-e2e-tests-infrastructure
depends_on: [epic-e2e-tests-infrastructure-module-skeleton]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Infra — ccdriver package + JSON contract test

## Scope

Implement `tests/e2e/fixtures/ccdriver/` — the Go package that
simulates Claude Code's hook lifecycle by invoking the `jamsesh`
binary's hook subcommands with crafted JSON stdin. Plus a frozen-
payload contract test asserting the driver's JSON shape matches
what Claude Code actually emits.

## Background

The `jamsesh` binary's hook subcommands (`hook session-start`,
`hook user-prompt-submit`, `hook pre-tool-use`, `hook post-tool-use`,
`hook stop`, `hook session-end`) read JSON from stdin and write
JSON to stdout. Real Claude Code invokes them with payload shapes
defined in the plugin's hook contract. The driver substitutes for
real Claude Code in e2e tests — deterministic, fast, no network.

## Files to create / modify

- `tests/e2e/fixtures/ccdriver/driver.go` — `Driver` type plus the
  six method signatures (`StartSession`, `SubmitPrompt`,
  `PreToolUse`, `PostToolUse`, `Stop`, `SessionEnd`)
- `tests/e2e/fixtures/ccdriver/payloads.go` — typed Go structs for
  each hook event's payload (matches Claude Code's wire format)
- `tests/e2e/fixtures/ccdriver/contract_test.go` — golden-file tests
  that compare driver-emitted JSON to frozen reference payloads
- `tests/e2e/fixtures/ccdriver/contract/{session-start,
  user-prompt-submit, pre-tool-use, post-tool-use, stop,
  session-end}.json` — frozen reference JSON for each hook event
- `tests/e2e/fixtures/ccdriver/README.md` — explains the
  contract-drift mitigation pattern

## Acceptance criteria

- [ ] `cd tests/e2e && go test ./fixtures/ccdriver/...` runs green
- [ ] The contract test fails loudly when a payload struct's
      generated JSON drifts from its frozen reference
- [ ] Each of the six hook events has a frozen-payload file and a
      contract subtest
- [ ] The driver's API is minimal — six exported methods + the
      `Driver` struct, plus typed payload structs; no exported
      helpers (those are internal)
- [ ] `README.md` documents how to refresh a frozen payload when
      the protocol intentionally changes (one-line `go test -update`
      pattern via a test flag, OR documented manual procedure)

## Notes for the implementer

- The driver does NOT need to actually invoke the `jamsesh` binary
  in this story's contract test — the test verifies the JSON shape
  emitted by the driver's payload builders. Integration with a real
  jamsesh binary happens in later e2e specs (golden-path feature)
- Source of truth for the hook payload shapes: the plugin's
  `hooks/hooks.json` and the implementation in `cmd/jamsesh/hook*/`.
  Read those files to derive the frozen payloads
- The contract test's failure message should be actionable: when a
  payload drifts, the test should diff the actual vs. expected JSON
  and point to the contract file to update if the drift is intentional
- Use the `testing.TB.TempDir()` pattern for `CLAUDE_PLUGIN_DATA` —
  the driver creates a clean per-test data dir

## Implementation notes

Files created:
- `tests/e2e/fixtures/ccdriver/driver.go` — `Driver` struct with six exported methods (`StartSession`, `SubmitPrompt`, `PreToolUse`, `PostToolUse`, `Stop`, `SessionEnd`); internal generic `runHook[I, O any]` handles subprocess invocation
- `tests/e2e/fixtures/ccdriver/payloads.go` — exported mirrors of the six private `<hook>Input`/`<hook>Output` types from `cmd/jamsesh/hooks/`; `PreToolUseInput.ToolInput` and `PostToolUseInput.ToolInput` are `json.RawMessage` matching the binary's wire format
- `tests/e2e/fixtures/ccdriver/contract_test.go` — `package ccdriver_test` (external); single `TestPayloadContracts` with six `t.Run` subtests; supports `-update` flag to regenerate frozen files; failure messages include got/want diff and the path to update
- `tests/e2e/fixtures/ccdriver/contract/session-start.json` — frozen payload
- `tests/e2e/fixtures/ccdriver/contract/user-prompt-submit.json` — frozen payload
- `tests/e2e/fixtures/ccdriver/contract/pre-tool-use.json` — frozen payload (includes `tool_input` with re-indented JSON object)
- `tests/e2e/fixtures/ccdriver/contract/post-tool-use.json` — frozen payload (includes `tool_input` and `tool_response`)
- `tests/e2e/fixtures/ccdriver/contract/stop.json` — frozen payload
- `tests/e2e/fixtures/ccdriver/contract/session-end.json` — frozen payload
- `tests/e2e/fixtures/ccdriver/README.md` — contract-drift mitigation pattern, wire-format table, usage example

Deviations from story body:
- `ToolResponse` is an exported type in `payloads.go` (required since `PostToolUseInput` embeds it and tests construct it directly); story body did not mention it explicitly but it follows naturally from the `postToolUseInput` private type
- Contract test uses `package ccdriver_test` (external test) with an explicit import, consistent with the project's test style noted in the story

Verification:
- `cd tests/e2e && go test ./fixtures/ccdriver/... -v` → all six subtests PASS
- `git diff go.mod` → empty (root module untouched)

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**:
- `runHook` env construction omits `os.Environ()` — `cmd.Env` starts empty, so subprocess has no `PATH`, `HOME`, etc. Latent bug because the contract test doesn't invoke the binary; will block any golden-path test that drives `jamsesh hook user-prompt-submit` (which needs `git` on PATH). Filed as `ccdriver-subprocess-env-inheritance` in `.work/backlog/` — golden-path design will pick it up.

**Nits**:
- Frozen JSON files lack trailing newlines (`\ No newline at end of file` per git diff). Cosmetic; tools usually add them, but the contract test compares exact bytes either way.
- `runHook` doesn't capture stderr separately — on subprocess failure, the caller gets `*exec.ExitError` but no stderr text for debugging. `cmd.Output()` only returns stdout. Consider `cmd.CombinedOutput()` or a dedicated `cmd.Stderr = &buf` for diagnosability.

**Notes**: The driver surface is minimal and symmetric — six methods, six payload types, six frozen JSON files. The `-update` flag pattern on the contract test is idiomatic Go for golden-file tests. Generic `runHook[I, O any]` deduplicates the boilerplate cleanly.
