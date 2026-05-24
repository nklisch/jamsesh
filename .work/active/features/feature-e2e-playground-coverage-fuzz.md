---
id: feature-e2e-playground-coverage-fuzz
kind: feature
stage: implementing
tags: [testing, e2e-test, playground, portal, fuzz]
parent: epic-e2e-playground-coverage
depends_on: [feature-e2e-playground-coverage-golden]
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Fuzz e2e tests for the playground subsystem

## Brief

Fuzz coverage for the v0.4.0 playground subsystem — specifically, the
nickname input boundary on `POST /api/playground/sessions/{id}/join`.
The unit-level table-driven test
`TestJoinPlaygroundSession_NicknameValidation` (added in story
`gate-tests-join-nickname-server-side-validation` during v0.4.0) covers
the documented rule "2-24 chars, letters/digits/dashes" with 6 invalid
and 5 valid cases against the in-process handler. The e2e fuzz harness
proposed here exercises the same surface against the real portal binary
+ real chi router + real openapi-validator middleware, catching any
divergence between the unit-suite's stubbed validation and the
production pipeline.

Existing fuzz harnesses at `tests/e2e/fuzz/` (4 harnesses covering
`fencing_token`, `mcp_tool_input`, `object_storage_dsn`,
`pack_manifest`) supply the harness pattern.

## Child stories

This feature has 1 child story, carried over from the
`e2e-test-design --audit` run:

1. `e2e-audit-playground-nickname-fuzz` (Low) — Go fuzz harness at
   `tests/e2e/fuzz/playground_nickname_test.go` that fuzzes the
   nickname-validation boundary against the real portal; asserts that
   every rejected input returns 400 with the documented error envelope
   and every accepted input returns 200 with the echoed nickname (or
   server-generated one if collision retry kicked in)

## Design status

Audit-supplied sketch is the seed. e2e-test-design's job is to lock the
fuzz corpus (start from the unit-test's 6+5 cases, then extend with
Unicode boundary cases, length boundary cases at 1/2/24/25, and
mixed-ASCII edge cases) and the assertion shape (echo-back exact match
vs. echo-back with collision-retry suffix).

## Mock-boundary plan

Inherited from golden. No new fixtures needed.

## Taxonomy plan

This feature is the **fuzz** layer for the playground. 1 Go fuzz
harness covering the nickname-validation boundary.

## Implementation Units

### Unit 1: nickname fuzz harness
**File**: `tests/e2e/fuzz/playground_nickname_test.go`
**Story**: `e2e-audit-playground-nickname-fuzz` (Low)
**Invariant**: For every fuzz-generated nickname input passed to
`POST /api/playground/sessions/{id}/join`:
- Inputs matching the documented rule (2-24 chars,
  letters/digits/dashes) return 200 with the echoed nickname (or a
  collision-retry suffix if the wordlist collided).
- All other inputs return 400 with `playground.invalid_nickname`.

Seed corpus from the unit-suite cases in
`TestJoinPlaygroundSession_NicknameValidation` (6 invalid + 5 valid).
Extend with Unicode boundary cases, length boundary cases at 1/2/24/25,
empty string (server-mints), mixed-ASCII edge cases.

## Anti-tautology guardrails

Inherited from golden. Fuzz inputs are random; assertions are on the
documented contract — never on "whatever the code returned". The
contract is: 2-24 chars letters/digits/dashes accepted with echo,
everything else rejected with the documented error code.

## Status

Design complete. Feature advanced to `implementing`. The 1 child story
is also advanced to `implementing`.

## Next

`/agile-workflow:implement-orchestrator feature-e2e-playground-coverage-fuzz`
— single-agent run.
