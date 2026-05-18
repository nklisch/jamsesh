---
id: epic-e2e-cnd-coverage-routing-layer-golden-mcp-header
kind: story
stage: review
tags: [e2e-test, testing, portal, infra]
parent: epic-e2e-cnd-coverage-routing-layer
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Golden: MCP Jam-Session-Id Header Pinning

## Scope

Implement `tests/e2e/golden/router_mcp_session_header_test.go`.

One subtest: **`mcp_jam_session_id_pins_to_handshake_pod`**

Steps:
1. Start a 2-pod cluster with the router fronting it.
2. Perform the full MCP initialize handshake against `c.RouterURL` using
   `mcpclient.New(t, c.RouterURL, bearer)`. The handshake sends a POST to
   `/mcp` — a non-session path, so the router round-robins. The portal returns
   a jamsesh session ID in its tool results (or the test creates a jamsesh
   session via REST first to obtain a known `session_id`).
3. After the MCP init, obtain the jamsesh `session_id` that the MCP session
   corresponds to. The test creates the jamsesh session via REST before calling
   MCP init, so `session_id` is known upfront.
4. Make N (≥5) subsequent MCP tool calls (`QuerySessionState` or `PostComment`)
   that include the `Jam-Session-Id: <session_id>` header. The router extracts
   this header in `extract.SessionID` (verified in extract package) and uses it
   for consistent-hash routing.
5. After each tool call, assert `cluster.LeaseHolder(ctx, t, session_id)`
   returns the same pod index as on the first call that established the lease.

**Header note**: The router uses `Jam-Session-Id` (not `Mcp-Session-Id`) for
MCP session extraction (verified at `internal/router/extract/extract.go`). The
`mcpclient` fixture sends `Mcp-Session-Id` (the MCP wire protocol session ID)
and does NOT currently send `Jam-Session-Id`. The test must either:
- Send `Jam-Session-Id` manually in a custom `http.Client` wrapping the
  mcpclient's transport, OR
- Make direct HTTP calls to `/mcp` with both `Mcp-Session-Id` and
  `Jam-Session-Id: <jamsesh_session_id>` headers set.

The simplest approach: build a thin helper `routerMCPRequest(t, routerURL,
bearer, mcpSessionID, jamSessionID, payload)` that sets both headers and posts
to `/mcp`. This sits in the test file (not a shared fixture) since it is
specific to this scenario.

## Invariant

> MCP tool calls carrying `Jam-Session-Id: <session_id>` are routed to the pod
> that holds the lease for that session, regardless of which pod served the
> MCP initialize handshake.

## Setup

```go
pg  := postgres.Start(ctx, t, postgres.Options{})
mn  := minio.Start(ctx, t, minio.Options{})
c   := portalcluster.Start(ctx, t, portalcluster.Options{
    Pods:        2,
    Postgres:    pg,
    ObjectStore: mn,
    Router:      true,
})
// Create jamsesh session via REST to get session_id
// Perform MCP init via router URL
// Make N MCP tool calls with Jam-Session-Id header
// Assert LeaseHolder == same pod each time
```

## Assertion targets

- `cluster.LeaseHolder(ctx, t, jamseshSessionID)` returns the same index for
  all N MCP tool calls (the pod that holds the lease for that session).
- All N tool calls return 200 with valid MCP response envelopes.

## Acceptance criteria

- [ ] Subtest `mcp_jam_session_id_pins_to_handshake_pod` passes.
- [ ] Routing identity verified via LeaseHolder, not just response status.
- [ ] Both `Jam-Session-Id` and `Mcp-Session-Id` headers set correctly in test
      HTTP calls so portal MCP session continuity is maintained.
- [ ] No in-process router or portal mocks.

## Implementation notes

- **File**: `tests/e2e/golden/router_mcp_session_header_test.go`
- **Accessor added**: `mcpclient.Client.MCPSessionID()` exported in
  `tests/e2e/fixtures/mcpclient/mcpclient.go` to expose the wire-protocol
  Mcp-Session-Id after the initialize handshake.
- **Header strategy**: `routerMCPRequest` (file-local helper) sets both
  `Mcp-Session-Id` (MCP SDK session continuity) and `Jam-Session-Id` (router
  extraction key, verified in `internal/router/extract/extract.go`).
- **Lease oracle**: `cluster.RequireLeaseHolder` (5 s timeout) waits for the
  advisory lock after REST session creation; subsequent `cluster.LeaseHolder`
  calls confirm every tool call lands on the same pod.
- **Tool used**: `query_session_state` — read-only and idempotent, safe for
  repeated calls without side-effects.
- **No mocks**: cluster of 2 real portal containers + 1 real router container
  via Testcontainers-Go. Docker image required; test skips automatically if
  unavailable (propagated from portal.Start).

## Test-integrity rules

- **Park production bugs, don't hide them.** If MCP requests with a valid
  `Jam-Session-Id` header are not being routed consistently (LeaseHolder
  oscillates), park the bug; land the test with `t.Skip`.
- **Never game an assertion.** A test that only checks response status (2xx)
  without verifying LeaseHolder is tautological for this feature — do not
  ship it.
