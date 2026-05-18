---
id: gate-security-ws-bearer-token-ticket-flow
kind: story
stage: backlog
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
