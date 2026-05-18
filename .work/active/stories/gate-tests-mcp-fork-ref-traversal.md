---
id: gate-tests-mcp-fork-ref-traversal
kind: story
stage: implementing
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
