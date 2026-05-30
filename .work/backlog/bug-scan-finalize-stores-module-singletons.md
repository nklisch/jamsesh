---
id: bug-scan-finalize-stores-module-singletons
created: 2026-05-30
tags: [bug, async, high]
bug_origin: scan
bug_severity: high
bug_domain: async
bug_location: frontend/src/lib/finalize/useFinalizeLock.svelte.ts:18
---

# Finalize rune stores are module-level singletons holding per-session state

**Location**: `frontend/src/lib/finalize/useFinalizeLock.svelte.ts:18` · **Severity**: high · **Pattern**: async handler racing component unmount / shared mutable state across async

Four finalize stores (`useFinalizeLock`, `useFinalizePlan`, `useFinalizeCuration`, `useFinalizeExecution`) are module singletons, unlike the per-instance factories used elsewhere (`createTreeState`, `createPlaygroundCountdown`). When a FinalizeView for session A unmounts while one for session B mounts (SPA route swap), B's `onMount` reset/acquireLock runs against the same `_lock`/`_plan`/`_selectedShas` while A's `onDestroy` reads `finalizeLock.status` (now B's) and may release the wrong lock; in-flight `acquireLock` awaits resolving after `onDestroy` also write into the singleton with no liveness guard. Fix: convert the four stores to per-instance factories (`createFinalizeLock(...)`), or thread an instance/sequence token through every async write and only release the lock whose `lock_id` matches the captured one.

```ts
// module scope, NOT a factory:
let _lock = $state<LockStatus | null>(null);
// onDestroy reads the singleton that B may now own:
if (finalizeLock.status && ...) void finalizeLock.release(orgId, sessionId);
```
