---
id: bug-squash-countdownbadge-per-tick-write
kind: story
stage: review
tags: [bug, ui, async]
parent: epic-bug-squash-frontend-async-races
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
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

## Implementation notes

Per the codex must-fix: a changed-value guard is a no-op at 1 tick/sec. Instead
relocated the remaining-time derivation and the 1-second setInterval to
`createPlaygroundCountdown` (`usePlaygroundCountdown.svelte.ts`). The store now
holds a `_now = $state(Date.now())` updated by the interval and the Page
Visibility API handler (previously in CountdownBadge), and derives
`idleRemainingMs`/`hardCapRemainingMs` via `$derived` from `_now` and the
deadline dates.

`CountdownBadge` is now display-only: it accepts `idleRemainingMs` and
`hardCapRemainingMs` as props and formats/renders them. The `onremainingupdate`
callback and the `now` clock are gone from the badge. The `updateRemaining`
method on the store (previously called by the callback) is also removed since
the store self-derives these values.

`SessionViewShell` passes `playground.idleRemainingMs` and
`playground.hardCapRemainingMs` directly to `CountdownBadge` instead of the
callback. Tests updated to use the new display-only prop interface.
