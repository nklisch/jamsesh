---
id: gate-tests-mcp-fuzz-seed-corpus
kind: story
stage: drafting
tags: [testing, security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# MCP fuzz seed corpus has zero entries for `fork` and only happy-path for `query_session_state`

## Priority
Medium

## Spec reference
Items: `epic-e2e-tests-fuzzing-mcp-tool-input` +
`gate-security-mcp-fork-ref-name-validation`
Acceptance criterion: any JSON to the four MCP tools returns 2xx or 4xx,
never 5xx (panic / unhandled error).

## Gap type
missing test for adversarial (corpus coverage skew). The random fuzz
generator may produce some fork calls but carefully-crafted adversarial
inputs only land if RNG happens to produce them.

## Suggested test
Add 6-8 fork seed entries covering: `target_ref:"../../base"`,
`target_ref:"-rf"`, `target_ref:"foo\x00bar"`, `target_ref:""`,
`target_ref:"refs/heads/jam/" + sessionID + "/x/y/z/../base"`,
`target_ref:strings.Repeat("a", 10000)`. Same for `query_session_state`
(currently single-shape) and `resolve_comment` (unknown event IDs,
recursive IDs).

## Test location (suggested)
`tests/e2e/fuzz/testdata/mcp-seed-corpus.json`
