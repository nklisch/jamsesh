---
id: gate-security-ws-bearer-token-ticket-flow
kind: story
stage: done
tags: [security, portal, refactor]
parent: null
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Replace WS subprotocol bearer with short-lived ticket

Long-term remediation for ws-bearer-token-leakage. Add /api/auth/ws-ticket
endpoint returning a 60-second single-use ticket. SPA POSTs to get a
ticket and uses it in Sec-WebSocket-Protocol instead of the long-lived
bearer. Ticket store is short-TTL, single-use, scoped to one upgrade.

Replaces the operator-redaction requirement documented in
gate-security-ws-bearer-token-leakage (v0.1.0).

## Implementation notes

### Files changed

- **`docs/openapi.yaml`** — added `POST /api/auth/ws-ticket` endpoint and
  `WsTicketResponse` schema (`ticket: string`, `expires_in_seconds: int`).
- **`internal/api/openapi/server.gen.go`** — regenerated (adds
  `IssueWsTicket` to `StrictServerInterface`, generated request/response types).
- **`frontend/src/lib/api/types.gen.ts`** — regenerated (adds `issueWsTicket`
  operation and `WsTicketResponse` schema).
- **`internal/portal/wsgateway/clock.go`** — new: per-package `Clock`
  interface + `realClock{}` following the per-package-clock-interface pattern.
- **`internal/portal/wsgateway/tickets.go`** — new: `TicketStore` with
  `Issue`, `Consume`, `Start`, `Stop` and a janitor goroutine.
- **`internal/portal/wsgateway/ticket_handler.go`** — new: `WsTicketHandler`
  implementing `IssueWsTicket` from `StrictServerInterface`.
- **`internal/portal/wsgateway/gateway.go`** — replaced `Tokens tokens.Service`
  field with `Tickets *TicketStore`; changed auth from `jamsesh.bearer.<token>`
  to `jamsesh-ticket.<ticket>`.
- **`internal/portal/wsgateway/tickets_test.go`** — new: unit tests for
  TicketStore (issue, consume, double-consume, expired, concurrent).
- **`internal/portal/wsgateway/ticket_handler_test.go`** — new: HTTP handler
  tests (401 without bearer, 200 with bearer, ticket consumable, unique tickets).
- **`internal/portal/wsgateway/gateway_test.go`** — updated: all tests use
  tickets instead of bearer tokens; added `TestHandler_RawBearer_Returns401`,
  `TestHandler_ReusedTicket_Returns401`, `TestHandler_MissingSubprotocol_Returns401`.
- **`internal/portal/tokens/handlers_test.go`** — added `IssueWsTicket` panic
  stub to satisfy the full `StrictServerInterface`.
- **`internal/portal/accounts/handlers_test.go`** — same stub addition.
- **`internal/portal/auth/magic_link_test.go`** — same stub addition.
- **`internal/portal/auth/oauth_test.go`** — same stub addition.
- **`internal/portal/comments/service_test.go`** — same stub addition.
- **`internal/portal/sessions/handler_test.go`** — same stub addition.
- **`internal/portal/router/body_limits_api_test.go`** — same stub addition.
- **`internal/portal/logging/logging_test.go`** — updated `TestAccessLogNoWSBearerLeak`
  comment to reflect the ticket-flow era.
- **`cmd/portal/main.go`** — added `WsTicketHandler` to `combinedHandler`,
  `IssueWsTicket` delegation method, `wsTicketStore` construction/startup,
  `POST /auth/ws-ticket` route behind `BearerMiddleware`, updated `Gateway`
  construction to use `Tickets` instead of `Tokens`.
- **`frontend/src/lib/ws.svelte.ts`** — replaced `jamsesh.bearer.<token>`
  protocol with ticket-fetch flow: `open()` and `reopen()` now async, call
  `client.POST('/api/auth/ws-ticket', {})` before each new socket.
- **`frontend/src/lib/ws.test.ts`** — rewrote all tests to mock `client.POST`
  for ticket fetches; updated protocol assertions; added tests for ticket-fetch
  on subscribe, ticket uniqueness on reconnect.
- **`docs/SECURITY.md`** — removed "Proxy log redaction for WebSocket bearer
  tokens" bullet from operator responsibilities.
- **`docs/SELF_HOST.md`** — removed "Proxy log redaction for WebSocket bearer
  tokens" bullet and the entire "Important: WebSocket bearer token in proxy
  logs" guidance block (§10).
- **`docs/PROTOCOL.md`** — replaced bearer-token-in-subprotocol description
  and operator security note with ticket-flow description.

### Ticket store design

- **Storage**: `sync.Map[string]*ticket` where the value holds `*store.Account`
  and `expiresAt time.Time`.
- **Token format**: 32 bytes from `crypto/rand`, base64url-encoded (no
  padding). 256 bits of entropy.
- **TTL**: 60 seconds, hardcoded. Not configurable until there is an operator
  request.
- **Single-use enforcement**: `sync.Map.LoadAndDelete` is the atomic
  consume primitive. Go's `sync.Map` documentation guarantees that
  `LoadAndDelete` is atomic — two concurrent callers presenting the same
  token will only one succeed in the `loaded == true` branch.
- **Janitor**: a background goroutine sweeps every 30 seconds to reclaim
  memory from expired entries that were never consumed. Expired entries
  are also rejected at consume time regardless of whether the janitor has
  run.
- **Clock injection**: `NewTicketStoreWithClock(clk Clock)` for test-only
  clock advancement; `NewTicketStore()` uses `realClock{}` in production.

### Protocol discriminator

Old format: `jamsesh.bearer.<token>` (bearer in Sec-WebSocket-Protocol)
New format: `jamsesh-ticket.<ticket>` (ticket in Sec-WebSocket-Protocol)

The gateway's `strings.CutPrefix` check changed from `"jamsesh.bearer."` to
`"jamsesh-ticket."`. There is **no backwards-compat path** — raw bearer format
is rejected with 401.

### Single-use enforcement (atomicity)

`sync.Map.LoadAndDelete` is used to atomically delete-and-return the ticket
entry. This is tested by `TestTicketStore_ConcurrentConsume` which races 20
goroutines against a single ticket and verifies exactly one wins.

**Limitation noted**: `TestTicketStore_ConcurrentConsume` verifies the
`sync.Map.LoadAndDelete` contract empirically but cannot guarantee the OS
scheduler will actually interleave the goroutines. The test has run
consistently as expected in the development environment. If the atomicity ever
became a concern in a shared-store (Redis/Postgres) cluster scenario, the
cluster design would need its own atomic-consume story.

### Removal of operator-redaction note

The following docs sections were removed because the bearer token no longer
appears in `Sec-WebSocket-Protocol`:

- `docs/SECURITY.md` §"Self-host security posture": removed the bullet
  "Proxy log redaction for WebSocket bearer tokens" and its sub-options.
- `docs/SELF_HOST.md` §10: removed the bullet from operator responsibilities
  and the entire "Important: WebSocket bearer token in proxy logs" block
  (NGINX/Envoy/Caddy/CloudFront/ALB redaction guidance).
- `docs/PROTOCOL.md` §"WebSocket event types": replaced bearer-in-header
  description and operator security note with ticket-flow description.

### SPA design

`open()` and `reopen()` are now `async`. The `subscribe()` call returns
synchronously and fires `open()` as a fire-and-forget `void` Promise. This
matches the existing pattern where the caller registers handlers and the socket
opens asynchronously. The `records.get(sessionId)` re-check after `await
fetchTicket()` guards against the case where `close()` was called while the
ticket fetch was in flight.

### Token revocation trade-off (documented)

If a bearer token is revoked via `POST /api/auth/revoke` after a ws-ticket
has been issued for that account but before it is consumed, the ticket is still
valid — it carries the already-resolved `*store.Account` snapshot. Given the
60-second TTL, the maximum exposure window is 60 seconds after revocation
before any already-issued tickets expire. This is documented here as the
accepted trade-off for the in-memory, single-process design.

### Clock skew

In-process clock only — no skew possible. Relevant only if the ticket store
is ever moved to a shared external store (Redis, Postgres), which is a separate
story.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Ticket carries a resolved `*store.Account` snapshot rather than the bearer
  itself, so a bearer revocation between issue and consume (≤60 s) leaves the
  ticket still valid. Documented as the accepted trade-off; worth revisiting if
  the project ever gains a stricter "instant revocation" guarantee.
- `TestTicketStore_ConcurrentConsume` is an empirical check; the actual
  guarantee is `sync.Map.LoadAndDelete`'s documented atomicity. The test is
  supplementary, which the implementation notes call out honestly.
- `TicketStore.Stop()` doesn't drain entries — fine for a 60-second TTL but
  worth keeping in mind if TTL ever grows.

**Notes**: Excellent execution on a substantial change. The protocol
discriminator switch (`jamsesh.bearer.` → `jamsesh-ticket.`) is clean and the
gateway tests explicitly cover RawBearer-rejected and ReusedTicket-rejected
paths, so the security regression surface is well-fenced. The 7 unrelated test
files that needed `IssueWsTicket` stub additions (because the new method
extends `StrictServerInterface`) were all updated rather than commented out.
SPA pattern (async `open`/`reopen`, fetch ticket before each connection, never
reuse) matches the security model exactly. PROTOCOL.md / SECURITY.md /
SELF_HOST.md all updated together so the operator-facing surface is consistent
with the code.
