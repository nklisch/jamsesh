---
id: bug-squash-ws-refetch-stale-overwrite
kind: story
stage: review
tags: [bug, ui, async]
parent: epic-bug-squash-frontend-sessionlist-subscription
depends_on: [epic-bug-squash-frontend-ws-lifecycle]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: medium
bug_domain: async
bug_location: frontend/src/lib/screens/SessionList.svelte:78
---

# Event-driven refetch handlers fire concurrent unsequenced fetches whose late responses clobber newer state

**Location**: `frontend/src/lib/screens/SessionList.svelte:78` (also `frontend/src/lib/components/TreeDag.svelte:60`) · **Severity**: medium · **Pattern**: stale-overwriting-fresh race on event-driven refetch

A burst of WS events (commit.arrived, merge.succeeded, ref.forked, mode.changed — common during active sessions) launches multiple overlapping GETs with no sequence guard. Responses can resolve out of order, so an older snapshot overwrites a newer one (`refs` in TreeDag, a session row in SessionList), showing stale data until the next event re-syncs. SessionList also only checks `data` and ignores `error`. Fix: add a per-fetch sequence guard (increment a counter before each fetch, bail on resolve if a newer fetch started) or debounce/coalesce the refetch, mirroring `useFinalizePlan._patchSeq`.

```ts
void client.GET('/api/orgs/{orgID}/sessions/{sessionID}', {...})
  .then(({ data }) => { if (data) updateSession(data); });  // no ordering guard
```

## Implementation notes

Fixed in `frontend/src/lib/screens/SessionList.svelte` (landed in the same commit as Unit 1,
implemented as a coherent change bundled in one worktree per design):

- Added `const refetchSeq = new Map<string, number>()` — component-local, bounded by SessionList
  lifetime; counters are monotonic and never reset on id reappearance.
- Added `refetchSession(id)` — bumps the per-session counter before each GET, bails silently on
  resolve if the counter has advanced (stale response); also checks `error` (failed GET leaves
  prior state intact, no overwrite).
- Added `bumpAndUpdate(id, patch)` — used by `session.finalizing` and `session.ended` handlers to
  atomically invalidate any in-flight GET before applying the status patch. This prevents a
  commit.arrived refetch that resolves AFTER session.ended from overwriting the terminal status.
- Handlers restructured via `makeHandler(id, type)` factory (also introduced in Unit 1).

Regression tests added in `frontend/src/lib/screens/SessionList.test.ts`:
- Two overlapping refetches: reverse-order resolution → only the newer applied.
- commit.arrived refetch in flight when session.ended fires → ended status NOT overwritten.
- Failed GET → existing state not overwritten.

All 764 vitest tests pass; svelte-check 0 errors.
