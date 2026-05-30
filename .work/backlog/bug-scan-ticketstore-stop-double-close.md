---
id: bug-scan-ticketstore-stop-double-close
created: 2026-05-30
tags: [bug, concurrency]
bug_origin: scan
bug_severity: medium
bug_domain: concurrency
bug_location: internal/portal/wsgateway/tickets.go:92
---

# TicketStore.Stop panics on a second call (double close of stopCh)

**Location**: `internal/portal/wsgateway/tickets.go:92` · **Severity**: medium · **Pattern**: double-close of channel

`Start` is idempotent (guards on `started`), but `Stop` closes `stopCh` without resetting `started=false`. A second `Stop()` — or a Start→Stop→Start→Stop lifecycle reusing the store — re-enters the close branch, and `close()` on an already-closed channel panics, crashing the process. Latent today (main calls Stop once) but contradicts the idempotent-Start design. Fix: guard the close with a `sync.Once`, or set `ts.started = false` after closing.

```go
func (ts *TicketStore) Stop() {
    ts.mu.Lock(); defer ts.mu.Unlock()
    if !ts.started { return }
    close(ts.stopCh)   // started never reset -> second Stop re-closes -> panic
}
```
