---
name: coder-websocket
description: Reference for github.com/coder/websocket (formerly nhooyr.io/websocket). Auto-loads when editing Go files that import github.com/coder/websocket, github.com/coder/websocket/wsjson, or nhooyr.io/websocket; when wiring the portal WebSocket gateway; or when building Sec-WebSocket-Protocol subprotocol-token upgrades. Triggers on terms — websocket.Accept, websocket.Dial, AcceptOptions, Sec-WebSocket-Protocol, Subprotocols, MessageText, MessageBinary, StatusNormalClosure, CloseRead, wsjson.Write, wsjson.Read.
user-invocable: false
---

# coder/websocket reference (jamsesh)

**Canonical import path**: `github.com/coder/websocket`. **Pinned
version**: v1.8.14 (2025-09-06).

**Rename history**: the project was `nhooyr.io/websocket` from
2019-2024; in 2024 it moved to Coder's stewardship and the canonical
path became `github.com/coder/websocket`. The old vanity import is
still resolvable but everything new should use the coder path.

**jamsesh foundation lag**: `docs/SPEC.md` and the API epic still
reference `nhooyr.io/websocket`. That's stale text — use
`github.com/coder/websocket` in new code and roll the foundation
docs forward when touched (per the rolling-foundation principle).

## Why this library (locked decision)

Modern context-aware API, zero dependencies, RFC 6455 + RFC 7692
compliant, concurrent writes, WebAssembly support. Smaller surface
than gorilla/websocket and built around `context.Context` cancellation
— which matches the rest of jamsesh's stack.

## jamsesh-specific upgrade pattern (subprotocol-token auth)

Browser WebSocket clients CANNOT set custom `Authorization` headers.
jamsesh encodes the bearer token in `Sec-WebSocket-Protocol`:

- **Client sends**: `Sec-WebSocket-Protocol: jamsesh.bearer.<token>`
- **Server validates**, then echoes the protocol back in
  `AcceptOptions.Subprotocols` (REQUIRED — otherwise the handshake
  fails per RFC 6455).
- Routing: `wss://portal/ws/sessions/<session_id>` — one connection,
  one session subscription.

```go
package wsgateway

import (
    "context"
    "net/http"
    "strings"

    "github.com/go-chi/chi/v5"
    "github.com/coder/websocket"
    "github.com/coder/websocket/wsjson"
)

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionID")

    proto := r.Header.Get("Sec-WebSocket-Protocol")
    token, ok := strings.CutPrefix(proto, "jamsesh.bearer.")
    if !ok {
        http.Error(w, "missing subprotocol token", http.StatusUnauthorized)
        return
    }
    acct, err := h.tokens.Validate(r.Context(), token)
    if err != nil {
        http.Error(w, "invalid token", http.StatusUnauthorized)
        return
    }
    if !h.members.IsMember(r.Context(), acct.ID, sessionID) {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }

    conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
        Subprotocols:   []string{proto},        // MUST echo exactly
        OriginPatterns: h.allowedOrigins,        // CSRF defense
        // CompressionMode: websocket.CompressionDisabled, // optional
    })
    if err != nil {
        return
    }
    defer conn.CloseNow()

    // Subscribe to the per-session fan-out registry.
    sub := h.registry.Subscribe(sessionID, conn)
    defer h.registry.Unsubscribe(sessionID, sub)

    // CloseRead returns a context cancelled when the peer disconnects
    // or sends a close frame. The conn becomes write-only.
    ctx := conn.CloseRead(r.Context())

    h.fanOut(ctx, conn, sub)
    conn.Close(websocket.StatusNormalClosure, "")
}
```

## Sending JSON envelopes

```go
type Envelope struct {
    Version   int             `json:"version"`     // always 1 for v1
    Seq       int64           `json:"seq"`         // per-session monotonic
    Type      string          `json:"type"`        // commit.arrived, etc.
    Payload   json.RawMessage `json:"payload"`
    Timestamp time.Time       `json:"timestamp"`
    SessionID string          `json:"session_id"`
}

func writeEnvelope(ctx context.Context, c *websocket.Conn, e Envelope) error {
    // wsjson.Write serializes + sends as MessageText in one call.
    return wsjson.Write(ctx, c, e)
}
```

`wsjson` lives at `github.com/coder/websocket/wsjson`.

## Heartbeats

```go
ticker := time.NewTicker(30 * time.Second)
defer ticker.Stop()
for {
    select {
    case <-ctx.Done():
        return
    case <-ticker.C:
        pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
        if err := conn.Ping(pingCtx); err != nil {
            cancel()
            return // client gone
        }
        cancel()
    case ev := <-sub.Events():
        if err := wsjson.Write(ctx, conn, ev); err != nil {
            return
        }
    }
}
```

`Ping` blocks until pong arrives (or ctx cancels). Use a per-ping
timeout — not the parent context — so a stalled pong doesn't kill the
whole connection.

## Close semantics

```go
conn.Close(websocket.StatusNormalClosure, "")   // graceful, sends close frame
conn.CloseNow()                                  // immediate, no handshake (use in defer)
```

Always `defer conn.CloseNow()` immediately after `websocket.Accept`
to guarantee cleanup on early return; a graceful `Close` later still
works.

## Pitfalls

- **Subprotocol echo**: any case/whitespace difference between the
  client's `Sec-WebSocket-Protocol` and the server's
  `AcceptOptions.Subprotocols` value fails the handshake. Use the
  raw header value verbatim.
- **OriginPatterns vs InsecureSkipVerify**: in production NEVER set
  `InsecureSkipVerify: true`. Populate `OriginPatterns` from config
  (e.g., `["portal.example.com"]`). The default origin check blocks
  cross-origin upgrades — correct for browser-hosted UIs.
- **Concurrent writes are safe** but concurrent reads are NOT. Have
  one goroutine reading and one writing; do not call `Read` from
  multiple goroutines.
- **`CloseRead` is one-way**: after calling it, the conn can only be
  written to. Use it when you don't expect inbound messages (the
  jamsesh fan-out model is server-push only).
- **`SetReadLimit`**: defaults to 32KiB. Bump for legitimately large
  payloads, but keep a cap to prevent memory-exhaustion attacks.
- **`CompressionMode`**: `CompressionContextTakeover` (default) is
  fine for typical JSON. Disable (`CompressionDisabled`) if your
  payloads are tiny and CPU-bound.
- **Status code on Close**: use defined `websocket.Status*` constants;
  custom codes outside 4000-4999 may be rejected by strict clients.
- **Don't import nhooyr.io/websocket in new code** — even though it
  still resolves, the dependency graph and module hash change. One
  canonical path per project.

## References

- API epic: `.work/active/epics/epic-portal-api.md`
  (websocket-gateway feature; subprotocol-token decision)
- Research doc: `docs/research/core-go-server-stack.md`
- godoc: https://pkg.go.dev/github.com/coder/websocket
- wsjson godoc: https://pkg.go.dev/github.com/coder/websocket/wsjson
- Repo: https://github.com/coder/websocket
- Coder stewardship post: https://coder.com/blog/websocket
