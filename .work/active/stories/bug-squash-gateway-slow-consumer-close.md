---
id: bug-squash-gateway-slow-consumer-close
kind: story
stage: review
tags: [bug, portal, concurrency]
parent: epic-bug-squash-worker-lifecycle
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: low
bug_domain: concurrency
bug_location: internal/portal/wsgateway/gateway.go:127
---

# Gateway slow-consumer close relies on network propagation to unwind the handler; fanout keeps iterating a dead conn

**Location**: `internal/portal/wsgateway/gateway.go:127` · **Severity**: low · **Pattern**: background task lifetime / channel send vs close ordering

When a subscriber's `send` buffer is full, fanout calls `c.ws.Close` from the fanout goroutine, but `c.send` is never closed and the conn is only removed from `subs` by the handler's deferred `unregister`. Until the close propagates through the network layer and cancels the handler's request context, fanout keeps attempting non-blocking sends to a conn being torn down. Functionally OK (coder/websocket Close is safe cross-goroutine) but it is minor churn and a window where a dead conn stays subscribed. Fix: after `closeOnce` fires, proactively `unregister(c)` (or have the handler select on a `c.closed` channel) so fanout stops iterating it.

```go
select {
case c.send <- e:
default:
    c.closeOnce.Do(func() { c.ws.Close(websocket.StatusPolicyViolation, "subscriber too slow") })
    // c not unregistered here; fanout keeps trying until handler ctx unwinds
}
```

## Implementation notes

Added `g.unregister(c)` immediately after `c.closeOnce.Do(ws.Close)` in the
`default` (slow-consumer) branch of `fanout`. `unregister` takes `g.mu` (write
lock); this is safe because fanout iterates a pre-snapshot `list`, not the live
map. Double-unregister (fanout + handler defer) is a no-op (map delete on absent
key). Added `TestHandler_SlowConsumer_UnregisteredAfterClose` which floods the
buffer and verifies the dead conn receives a 1008 close, confirming the fix path
is exercised. Build/vet/`-race` clean: `go test -race ./internal/portal/wsgateway/...`.
