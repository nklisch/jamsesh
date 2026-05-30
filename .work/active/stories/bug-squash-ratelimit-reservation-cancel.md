---
id: bug-squash-ratelimit-reservation-cancel
kind: story
stage: drafting
tags: [bug, portal, concurrency]
parent: epic-bug-squash
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: low
bug_domain: concurrency
bug_location: internal/portal/ratelimit/store.go:106
---

# Rate-limit Allow does not cancel the minute reservation on the !OK early return; two-limiter accounting is non-atomic

**Location**: `internal/portal/ratelimit/store.go:106` · **Severity**: low · **Pattern**: atomicity violation across multiple atomic ops

When the minute limiter's `ReserveN` returns `!OK()` the code returns without `r.Cancel()`, inconsistent with every other branch (benign because `rate.Limiter` makes a `!OK` reservation a no-op, but inconsistent). More broadly the minute/hourly pair is reserved non-atomically, so concurrent callers contend across the two limiters and the accounting is only approximately correct. Low blast radius (auth rate limit, fail-safe direction). Fix: cancel `r` consistently on every early return; if exactness matters, hold `s.mu` across both Reserve calls.

```go
r := e.minuteLimiter.ReserveN(now, 1)
if !r.OK() { return false, 60*time.Second }   // r not cancelled (other branches do r.Cancel())
```
