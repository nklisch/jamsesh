---
id: epic-e2e-tests-fuzzing-mcp-tool-input
kind: story
stage: review
tags: [e2e-test, testing, portal]
parent: epic-e2e-tests-fuzzing
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Fuzzing — MCP tool input schemas

## Scope

A property-based fuzz harness for the four MCP tools (`post_comment`,
`resolve_comment`, `fork`, `query_session_state`). The harness drives
real HTTP POSTs to the portal's `/mcp` endpoint with generated JSON
bodies and asserts that:

1. Any generated JSON either validates and yields a typed response OR
   validates-fails and yields an MCP-error response
2. The portal never reaches a handler with malformed data
3. The portal never panics on input

## Approach

The fuzzer is property-based, not coverage-based — Go's stdlib
`go test -fuzz` is coverage-based and doesn't compose well with HTTP
calls. Instead, use a property-based generator (e.g.,
`github.com/leanovate/gopter`) that produces structured JSON shapes
covering:

- Valid shapes (all required fields, valid types)
- Missing required fields (each one)
- Wrong types (string instead of int, array instead of object, etc.)
- Boundary values (empty strings, max-length strings, huge integers)
- Encoding variations (unicode, null bytes, control chars)

For each generated body, POST to `/mcp` with proper bearer auth, then
classify the response:
- 2xx with valid tool response → input was valid
- 4xx with MCP error envelope → input was invalid (expected)
- 5xx → BUG; capture the body and file a backlog story

## Files to create / modify

- `tests/e2e/fuzz/mcp_tool_input_test.go` (NEW) — property-based
  test that drives the real `/mcp` endpoint
- `tests/e2e/fuzz/testdata/mcp-seed-corpus.json` — hand-curated seed
  inputs (real production examples + known-tricky cases)
- `Makefile` — `test-fuzz` target updated to also run
  `cd tests/e2e && go test ./fuzz/ -run TestMCPToolInputFuzz -count=N`
  where N is a CI-appropriate iteration count

## Acceptance criteria

- [ ] `cd tests/e2e && go test ./fuzz/ -v -count=100` runs without
      any 5xx responses (i.e., no panics, no internal server errors
      surface to the client)
- [ ] At least 10 hand-curated seed inputs are checked in (real
      production shapes, known-tricky boundaries, common attack
      patterns)
- [ ] The property is stated in plain English at the top of the
      test
- [ ] On any 5xx response, the test fails loudly with the offending
      input and response body so the implementor can reproduce
- [ ] No new go.mod deps in the root module; the `gopter` (or
      equivalent) dep is added to `tests/e2e/go.mod` only

## Notes for the implementer

- Per the user directive: any 5xx response IS a production bug —
  the portal should never panic on input. File the offending seed
  as a backlog story (`tags: [bug, portal]`) and either continue
  with the test failing (documenting why) or fix the bug inline
  if it's small.
- The MCP endpoint requires bearer auth. Use `authflow.SignInViaMagicLink`
  to get a real token before the property loop starts.
- The MCP protocol is JSON-RPC-like; the request shape is documented
  in `docs/openapi.yaml > Mcp-Session-Id`. Inspect the actual
  request format from the SDK's wire output if needed.
- For the four tools, each has its own request body shape; the
  fuzzer should generate inputs for ALL FOUR shapes. The simplest
  approach is a per-tool sub-test inside `TestMCPToolInputFuzz`.

## Risks

- The MCP SDK's streamable-http transport adds framing that the
  fuzzer must respect. Easiest approach: use the same `mcpclient`
  fixture from golden-path's `collab-merge` story IF that lands
  first. Otherwise, use raw HTTP POSTs with hand-crafted MCP
  envelopes.
- Property-based testing can produce huge numbers of inputs;
  ensure CI runs a bounded count. The harness should be runnable
  with a longer count for nightly fuzz jobs.

## Implementation notes

- Created `tests/e2e/fuzz/mcp_tool_input_test.go` with
  `TestMCPToolInputFuzz`. Uses `callRawTool` (hand-rolled) that
  performs the MCP initialize handshake + `tools/call` POST per
  iteration, so each call is fully independent.
- Created `tests/e2e/fuzz/testdata/mcp-seed-corpus.json` with 22
  hand-curated cases covering: valid shapes for all four tools,
  missing required fields, wrong types, huge strings, SQL
  injection, path traversal, unicode/null bytes, empty bodies, and
  an unknown tool name.
- The harness runs two phases:
  1. All seed corpus entries as deterministic subtests
     (`seed_NN_<description>`).
  2. N random property iterations using `math/rand/v2` with a
     logged seed for reproducibility. Default N=200; override via
     `MCP_FUZZ_COUNT`. Reproduce via `MCP_FUZZ_SEED=<seed>`.
- `callRawTool` embeds raw `json.RawMessage` verbatim in the
  arguments field, bypassing typed marshalling — this is
  intentional so wrong-type and null inputs reach the portal
  unmodified.
- No new root-module deps; the harness is contained entirely in
  `tests/e2e` which has its own `go.mod`.
- Makefile target added: `make test-fuzz-mcp` runs the harness
  with `-timeout 300s`.
- The test is skipped under `go test -short`.
