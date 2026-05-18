---
id: gate-tests-ws-bearer-redact
kind: story
stage: implementing
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
