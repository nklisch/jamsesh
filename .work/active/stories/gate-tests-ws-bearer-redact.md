---
id: gate-tests-ws-bearer-redact
kind: story
stage: done
tags: [testing, security, portal]
parent: null
depends_on: [gate-security-ws-bearer-token-leakage]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# WebSocket bearer-token-in-subprotocol — no test that access logs redact it

## Priority
High

## Spec reference
Item: `gate-security-ws-bearer-token-leakage`
Acceptance criterion: either operators must strip/redact
`Sec-WebSocket-Protocol` from upstream proxy access logs, or the portal
switches to a short-lived ticket flow.

## Gap type
missing test for adversarial-spec-silent.
`internal/portal/logging/logging_test.go` contains zero references to
`Sec-WebSocket-Protocol`, `bearer`, or `redact`.

## Suggested test
```go
// TestAccessLog_WSUpgrade_DoesNotContainBearerToken
//   Open a WS connection with Sec-WebSocket-Protocol: jamsesh.bearer.<long-token>.
//   Capture the access-log JSON. Assert: log payload does NOT include the token,
//   and the header field if logged is redacted to "jamsesh.bearer.[REDACTED]".
```

## Test location (suggested)
`internal/portal/logging/logging_test.go` (or new
`wsgateway/access_log_test.go`)

## Implementation notes

Added `TestAccessLogNoWSBearerLeak` in `internal/portal/logging/logging_test.go`.

The test:
- Sets up `Access(nil)` middleware with a JSON slog handler writing to a `bytes.Buffer`.
- Issues a GET request with `Sec-WebSocket-Protocol: jamsesh.bearer.SECRET_TOKEN_123`.
- Asserts the raw log line does NOT contain `SECRET_TOKEN_123`.
- Also verifies that `method`, `path`, and `status` fields are present and correct.

Result: test passes — the access-log middleware logs only `method`, `path`, `route`,
`status`, `duration_ms`, and `bytes`. No request headers are logged; no bearer token
leakage occurs. This confirms the docs-only mitigation in `gate-security-ws-bearer-token-leakage`
is sufficient for the current implementation.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: TestAccessLogNoWSBearerLeak added. Captures slog JSON output, fires GET with Sec-WebSocket-Protocol: jamsesh.bearer.SECRET_TOKEN_123, asserts SECRET_TOKEN_123 substring is absent. Confirms access-log middleware logs only method/path/route/status/duration_ms/bytes — no request headers, no token leakage. The docs-only mitigation in gate-security-ws-bearer-token-leakage is sufficient for the current implementation.
