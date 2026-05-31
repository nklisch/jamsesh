---
id: bug-squash-sessionlist-resubscribe-churn
kind: story
stage: done
tags: [bug, ui, state]
parent: epic-bug-squash-frontend-sessionlist-subscription
depends_on: [epic-bug-squash-frontend-ws-lifecycle]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: medium
bug_domain: state
bug_location: frontend/src/lib/screens/SessionList.svelte:68
---

# SessionList WS-subscription $effect reads the same `sessions` array its handlers reassign

**Location**: `frontend/src/lib/screens/SessionList.svelte:68` · **Severity**: medium · **Pattern**: effect writes (indirectly) state it also reads — reactive dependency feedback

The subscription `$effect` iterates `sessions`, so it depends on that array. Every WS event calls `updateSession`, which **reassigns** `sessions`, re-running the effect and tearing down/re-subscribing every per-session subscription from scratch (4·N subscriptions recreated per event); presence/commit events also fire a refetch that calls `updateSession` again, compounding the churn. Not an infinite loop and events are not dropped (teardown+resubscribe is synchronous), but it is wasted work that scales with list size and event rate. Fix: key the effect on a stable derived of session IDs (`sessions.map(s => s.id).join(',')`) so it re-runs only when the set of sessions changes, or move subscription wiring into `onMount` keyed by the loaded ID set.

```ts
$effect(() => {
  const unsubs = [];
  for (const s of sessions) { /* subscribe(...); handlers call updateSession -> reassigns sessions */ }
  return () => { for (const u of unsubs) u(); };  // tears down ALL subs every event
});
```

## Implementation notes

Fixed in `frontend/src/lib/screens/SessionList.svelte`:

- Added `const sessionIdsKey = $derived(sessions.map((s) => s.id).sort().join(','))` — a stable
  string that changes only when the set of session ids changes (not on field updates).
- Changed `$effect` to read only `sessionIdsKey` as its reactive dependency; reads the actual
  session ids via `untrack(() => sessions.map(s => s.id))` so field-only `updateSession` calls
  (which reassign `sessions`) do NOT re-run the effect.
- Added `untrack` to the `svelte` import alongside existing `onMount`.
- Extracted `TYPES` constant and `makeHandler(id, type)` factory for clarity.
- The unsub-all/resub-all pattern (not a delta map) is correct because ws.svelte's macrotask
  linger absorbs the synchronous cleanup→resubscribe window; surviving sessions' sockets stay
  open because their handlers re-subscribe within the same synchronous effect re-run.

Regression tests added in `frontend/src/lib/screens/SessionList.test.ts`:
- Field-only event (commit.arrived) does not cause subscribe/unsubscribe churn.
- Adding a new session causes exactly its subscriptions to be added; surviving sockets not torn down.

All 764 vitest tests pass; svelte-check 0 errors.
