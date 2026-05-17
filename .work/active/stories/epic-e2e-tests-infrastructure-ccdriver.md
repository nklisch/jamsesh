---
id: epic-e2e-tests-infrastructure-ccdriver
kind: story
stage: implementing
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
