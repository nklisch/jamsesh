---
id: bug-scan-countdownbadge-per-tick-write
created: 2026-05-30
tags: [bug, async]
bug_origin: scan
bug_severity: low
bug_domain: async
bug_location: frontend/src/lib/components/CountdownBadge.svelte:50
---

# CountdownBadge $effect pushes onremainingupdate into the parent store every tick

**Location**: `frontend/src/lib/components/CountdownBadge.svelte:50` · **Severity**: low · **Pattern**: state update across async tick / cross-component per-tick write

The interval and visibility listener are correctly cleaned up in the `onMount` return, but the `$effect` calls `onremainingupdate(...)` on every `now` change (once per second), driving `playground.updateRemaining` and re-deriving downstream banner props — a benign-but-noisy per-second cross-component reactive write. No hard crash (the parent store is a per-instance factory tied to the shell), so latent/edge. Fix (optional): move the remaining-time math into the parent (it already holds `hardCapAt`/`idleTimeoutAt`) and drop the per-tick callback, or guard the callback behind a changed-value check.

```ts
$effect(() => { onremainingupdate?.(idleRemainingMs, hardCapRemainingMs); });  // fires every tick
```
