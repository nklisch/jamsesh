---
id: gate-security-ws-bearer-token-leakage
kind: story
stage: review
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

## Implementation notes

Took option (a): documentation update for v0.1.0. The ticket-flow refactor
is parked as a backlog item for a later release.

Docs changed:

- `docs/SECURITY.md` — added a bullet to the "Self-host security posture"
  operator-responsibilities list explaining the `Sec-WebSocket-Protocol`
  logging risk and the three remediation options (strip at proxy, omit from
  log format, terminate WS natively).

- `docs/SELF_HOST.md` §10 — added a "WebSocket bearer token in proxy logs"
  important notice block with proxy-specific guidance (NGINX `proxy_set_header`,
  Caddy `request_header`, log-format exclusion, native TLS termination).

- `docs/PROTOCOL.md` — added a security note in the WebSocket event types
  section explaining the subprotocol-header bearer mechanism and directing
  operators to SELF_HOST.md §10 for redaction options.

Follow-on parked at `.work/backlog/gate-security-ws-bearer-token-ticket-flow.md`
— short-lived ticket endpoint (`/api/auth/ws-ticket`) to replace the
long-lived-token-in-header flow entirely.
