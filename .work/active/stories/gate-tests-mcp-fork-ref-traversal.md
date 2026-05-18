---
id: gate-tests-mcp-fork-ref-traversal
kind: story
stage: review
tags: [testing, security, portal]
parent: null
depends_on: [gate-security-mcp-fork-ref-name-validation]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# MCP `fork` tool ref-name traversal has no test asserting rejection

## Priority
Critical

## Spec reference
Item: `gate-security-mcp-fork-ref-name-validation`
Acceptance criterion: a caller supplying `target_ref: "../../base"` must
be rejected, base ref must remain unclobbered.

## Gap type
missing test for adversarial-spec-silent. `mcpendpoint/handler_test.go:515-585`
covers happy-path and bad-commit only. `tests/e2e/fuzz/testdata/mcp-seed-corpus.json`
contains zero `"tool": "fork"` entries.

## Suggested test
```go
// TestMCPEndpoint_Fork_TargetRefTraversal_RejectedAndDoesNotMutateBaseRef
// Pre-seed refs/heads/jam/<sess>/base = sha_A.
// Call fork with target_ref="../../base", target_commit_sha=sha_B.
// Expect tool error 4xx; assert base ref still resolves to sha_A.
```

## Test location (suggested)
`internal/portal/mcpendpoint/handler_test.go` AND a fork-with-traversal
seed entry in `tests/e2e/fuzz/testdata/mcp-seed-corpus.json`.

## Implementation notes

**Assertion strategy:**

`TestMCPEndpoint_Fork_TargetRefTraversal_RejectedAndDoesNotMutateBaseRef` uses
a two-part assertion:
1. Pre-seeds `refs/heads/jam/<sessID>/base = sha_A` directly in the bare repo
   via `repo.Storer.SetReference` to simulate the state after the first
   receive-pack push.
2. Calls the fork MCP tool with `target_ref="../../base"` and
   `target_commit_sha=sha_B` (all-b's SHA, not present in the repo).
3. Asserts `IsError=true` in the MCP result envelope (validation rejection).
4. Re-opens the repo with `gogit.PlainOpen` (fresh storer — no cache) and
   reads back `refs/heads/jam/<sessID>/base`, asserting it still resolves to
   `sha_A`. This outcome assertion is the defence-in-depth check: even if the
   error path were somehow bypassed, the test would catch the clobber.

`TestMCPEndpoint_Fork_AdversarialTargetRef` is table-driven with 10 subtests
run in parallel, each in an isolated `newTestEnv`. Inputs: `../../base`,
`-rf`, `\x00`-containing, empty string, space, `\n`, `..base`,
`refs/heads/...`-prefixed deep traversal, `.hidden` (leading dot),
`branch.` (trailing dot). All 10 reject with `IsError=true`.

**No security holes found:** All 10 adversarial payloads were correctly
rejected by `validateForkTargetRef`. No amendments to `tools.go` were needed.

**Corpus entries added** to `tests/e2e/fuzz/testdata/mcp-seed-corpus.json`
(8 new `"tool": "fork"` entries):
- `../../base` — canonical traversal
- `-rf` — leading-dash injection
- `""` — empty string
- `..base` — dot-dot prefix without slash
- `refs/heads/jam/sess/x/y/z/../../../base` — slash-escaped deep traversal
- 10,000-char `a`-string — overlong name stress test
- `.hidden` — leading dot
- `foo bar` — space in name
