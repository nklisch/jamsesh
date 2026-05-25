# Context-Cancellable Ticker Sweep Loop

Long-running background goroutines (destruction worker, lease retention,
lease heartbeat, ticket janitor, object-store lifecycle manager) all share
the same minimal shape:
`ticker := time.NewTicker(interval); defer ticker.Stop(); for { select { case <-<stop>: return; case <-ticker.C: doOnePass() } }`.
The stop channel is whichever cancellation signal the goroutine is wired to
— `ctx.Done()`, a per-handle `done` channel, or a package-local `stopCh`.

## Rationale

Every portal-side background worker that does periodic cleanup or polling
needs identical lifecycle discipline: tick at a fixed interval, respond
promptly to shutdown, never leak the ticker. Inlining the loop (rather than
wrapping in a helper) keeps the body legible and makes the per-tick body
obvious — each worker's `doOnePass` is the interesting bit. The `defer
ticker.Stop()` is the line that gets forgotten when this isn't a recognized
pattern; calling it out as canonical makes it harder to omit.

## Examples

### Example 1: playground destruction sweep, ctx-cancelled

**File**: `internal/portal/playground/worker.go:71`

```go
ticker := time.NewTicker(w.Interval)
defer ticker.Stop()

const purgeEvery = 60
tick := 0

for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-ticker.C:
        tick++
        w.sweep(ctx)
        if tick%purgeEvery == 0 {
            w.purgeTombstones(ctx)
        }
    }
}
```

### Example 2: object-store lifecycle eviction, ctx-cancelled with shutdown drain

**File**: `internal/portal/storage/objectstore/lifecycle.go:333`

```go
ticker := time.NewTicker(m.idleCheckPeriod())
defer ticker.Stop()

for {
    select {
    case <-ctx.Done():
        m.shutdownAll(ctx)
        return ctx.Err()
    case <-ticker.C:
        m.evictIdleAndOversize(context.Background())
    }
}
```

### Example 3: ws-gateway ticket janitor, package-local stopCh

**File**: `internal/portal/wsgateway/tickets.go:135`

```go
ticker := time.NewTicker(ticketJanitorInterval)
defer ticker.Stop()
for {
    select {
    case <-ts.stopCh:
        return
    case <-ticker.C:
        now := ts.clock.Now()
        ts.entries.Range(func(k, v any) bool {
            if v.(*ticket).expiresAt.Before(now) {
                ts.entries.Delete(k)
            }
            return true
        })
    }
}
```

Also at `internal/portal/lease/retention.go:26` (RunRetention, ctx-cancelled)
and `internal/portal/lease/postgres.go:249` (heartbeat, per-handle `done`
channel). 5 occurrences total.

## When to Use

- A subsystem needs a periodic background sweep (cleanup, expiry, polling)
  and runs for the process lifetime.
- The per-tick work is bounded and idempotent — partial failure on tick N is
  corrected on tick N+1.
- A clean shutdown signal exists (ctx, stopCh, done channel).

## When NOT to Use

- One-shot deferred work — use `time.AfterFunc` instead.
- Per-request timeouts inside a handler — that's `context.WithTimeout`.
- Variable-interval scheduling where consecutive intervals depend on each
  other — `time.NewTimer` with manual Reset is more honest.

## Common Violations

- Forgetting `defer ticker.Stop()` — leaks a goroutine inside
  `time.Ticker.runtimeTimer` for every restart.
- Putting the shutdown channel inside the loop body rather than as a select
  case — the select-on-shutdown is the whole point.
- Sleeping with `time.Sleep(interval)` instead of a ticker — defeats the
  prompt-shutdown property; a `<-ctx.Done()` cannot interrupt a sleep.
- Burying the per-tick work directly in the select case rather than calling
  a named helper — makes the loop hard to test (you can't call
  `worker.sweep(ctx)` from a test without going through the loop).
