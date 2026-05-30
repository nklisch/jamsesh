---
id: bug-squash-ws-refetch-stale-overwrite
kind: story
stage: drafting
tags: [bug, ui, async]
parent: epic-bug-squash-frontend-async-races
depends_on: []
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
