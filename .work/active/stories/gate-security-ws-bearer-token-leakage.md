---
id: gate-security-ws-bearer-token-leakage
kind: story
stage: drafting
tags: [security, portal, documentation]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# WebSocket Bearer token in `Sec-WebSocket-Protocol` reaches access logs in plaintext

## Severity
Medium

## Domain
Data Protection

## Location
`internal/portal/wsgateway/gateway.go:142-147`,
`internal/portal/logging/logging.go:60-67`

## Evidence
```go
proto := r.Header.Get("Sec-WebSocket-Protocol")
token, ok := strings.CutPrefix(proto, "jamsesh.bearer.")
```

The bearer token rides in the `Sec-WebSocket-Protocol` request header.
The access-log middleware logs `path` and `route`, but the header itself
routinely lands in upstream reverse-proxy / load-balancer access logs
(NGINX/Envoy/CloudFront default formats include `$http_*`). Because the
token is the full access credential, log harvesting yields takeover.

## Remediation direction
Document explicitly that operators must strip/redact
`Sec-WebSocket-Protocol` from upstream proxy access logs, or switch to a
short-lived ticket flow: client POSTs to `/api/auth/ws-ticket`, gets a
60-second single-use ticket, presents that in the subprotocol header
instead of the long-lived access token.
