---
id: epic-bug-squash-frontend-sessionlist-subscription
kind: feature
stage: implementing
tags: [bug, ui]
parent: epic-bug-squash
depends_on: [epic-bug-squash-frontend-ws-lifecycle]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# SessionList subscription & event-refetch correctness

## Brief

The SessionList screen consumes the WebSocket manager's `subscribe` API and
refetches session rows on events. The bug-scan found two coupled defects here:
the subscription `$effect` reads the same `sessions` array its handlers
reassign, so every event tears down and re-subscribes all per-session
subscriptions (churn), and the event-driven refetch fires concurrent
unsequenced GETs whose late responses can clobber newer state
(stale-overwriting-fresh).

This feature delivers a stable subscription effect (keyed on the session-id set,
not the mutable array) and a sequence-guarded refetch. It covers SessionList's
subscription/refetch correctness only.

It **depends on `epic-bug-squash-frontend-ws-lifecycle`** because both fixes
build on that feature's corrected `subscribe`/`close` contract (ref-counted
teardown); landing them before the lifecycle rework would force rework. This was
split out from `frontend-async-races` per the codex decomposition gate so the
WS dependency does not block the independent async fixes.

## Epic context
- Parent epic: `epic-bug-squash`
- Position in epic: consumer of `epic-bug-squash-frontend-ws-lifecycle` (its
  corrected subscribe/close contract).

## Foundation references
- `docs/SPEC.md` — Svelte 5 SPA, EventEnvelope spec-driven types, openapi-fetch
- Patterns: `openapi-fetch-result-branch`, `wrapper-object-rune-store`

## Child stories (pre-existing, from bug-scan — re-parented here)
- `bug-squash-ws-refetch-stale-overwrite` — Medium, async — `frontend/src/lib/screens/SessionList.svelte:78`
- `bug-squash-sessionlist-resubscribe-churn` — Medium, state — `frontend/src/lib/screens/SessionList.svelte:68`

## Architectural choice

**Two coordinated fixes in `SessionList.svelte`**, building on the ws-lifecycle
ref-counted `subscribe`/`close` contract (this feature's `depends_on`). Same
file — bundle in one worktree. No new abstraction.

## Implementation Units

### Unit 1: Stabilize the subscription effect on the session-id SET
**File**: `frontend/src/lib/screens/SessionList.svelte`
**Story**: `bug-squash-sessionlist-resubscribe-churn` (Medium)

The `$effect` iterates `sessions`, so it depends on the whole array; every event
calls `updateSession` → reassigns `sessions` → re-runs the effect → tears down
and re-subscribes all 4·N subscriptions. Key the effect on a stable derived of
the id SET so it re-runs only when sessions are added/removed:

```ts
const sessionIdsKey = $derived(sessions.map((s) => s.id).sort().join(','));

$effect(() => {
  sessionIdsKey; // the ONLY reactive dependency — a field update doesn't change it
  const ids = untrack(() => sessions.map((s) => s.id));
  const unsubs: (() => void)[] = [];
  for (const id of ids) {
    for (const type of TYPES) unsubs.push(subscribe(id, type, makeHandler(id, type)));
  }
  return () => { for (const u of unsubs) u(); };
});
```

**Implementation Notes**: read `sessions` inside the effect via `untrack(...)`
so only `sessionIdsKey` is tracked. The handler bodies call `updateSession` /
the guarded refetch (Unit 2). On a genuine id-set change the effect re-runs and
unsubscribes the old set — with ws-lifecycle's ref-counted teardown, sockets for
removed sessions close (the macrotask linger absorbs the synchronous
unsub-all→resubscribe). `sort()` makes the key order-insensitive so reordering
doesn't churn.

**Acceptance Criteria** (codex clarification — this is unsub-all/resub-all +
linger, NOT a delta map):
- [ ] A field-only event (e.g. `commit.arrived` → `updateSession`) does NOT
      re-run the subscription effect (no unsubscribe/resubscribe churn).
- [ ] On a genuine id-set change, SURVIVING sessions' sockets stay open (their
      handlers re-subscribe within the same synchronous effect re-run, cancelling
      the ws-lifecycle teardown linger); REMOVED sessions' sockets close; ADDED
      sessions get fresh subscriptions. (Assert socket open/closed state, not a
      "only the new session subscribed" delta.)

Note: add `untrack` to the `svelte` import (the file currently imports only
`onMount`).

### Unit 2: Sequence-guarded per-session refetch
**File**: `frontend/src/lib/screens/SessionList.svelte`
**Story**: `bug-squash-ws-refetch-stale-overwrite` (Medium)

The event-driven refetch fires concurrent unsequenced GETs whose late responses
can clobber newer state, and only checks `data` (ignores `error`). Add a
per-session monotonic guard (mirrors `useFinalizePlan._patchSeq`):

```ts
const refetchSeq = new Map<string, number>();
function refetchSession(id: string) {
  const seq = (refetchSeq.get(id) ?? 0) + 1;
  refetchSeq.set(id, seq);
  void client.GET('/api/orgs/{orgID}/sessions/{sessionID}', { params: { path: { orgID: orgId, sessionID: id } } })
    .then(({ data, error }) => {
      if (refetchSeq.get(id) !== seq) return; // a newer refetch superseded this one
      if (data) updateSession(data);
      // else: leave prior state; optionally surface `error` (best-effort refresh)
    });
}
```

**Implementation Notes**: the guard is per-session (a Map keyed by id), so the
latest refetch for each session wins regardless of resolve order. Checking
`error` avoids silently treating a failed GET as success. **Codex must-fix**: the
status-event handlers (`session.finalizing`, `session.ended`) that call
`updateSession` directly MUST also bump `refetchSeq.get(id)` — otherwise a
`commit.arrived` refetch already in flight can resolve afterward and OVERWRITE
the ended/finalizing status with stale data. So any event-derived `updateSession`
for a session invalidates that session's outstanding GET:

```ts
function bumpAndUpdate(id: string, patch: Partial<Session> & { id: string }) {
  refetchSeq.set(id, (refetchSeq.get(id) ?? 0) + 1); // invalidate in-flight GETs for this id
  updateSession(patch);
}
// session.finalizing / session.ended handlers call bumpAndUpdate(...)
```

The `refetchSeq` Map is component-local and bounded by the SessionList lifetime;
keep counters monotonic (do NOT reset on id reappearance) so a returning id can't
accept a stale older response.

**Acceptance Criteria**:
- [ ] Two overlapping refetches for the same session: only the later-issued one's
      response is applied (earlier late response is dropped).
- [ ] A `commit.arrived` refetch in flight when `session.ended` fires does NOT
      overwrite the ended status (the ended handler bumped the seq).
- [ ] A failed GET does not overwrite existing session state.

## Implementation Order
Both units in SessionList.svelte — one coordinated change, bundle in one
worktree. Feature `depends_on` ws-lifecycle (the corrected subscribe/close
contract) at the feature level.

## Testing (vitest + jsdom)
- Unit 1: mock `subscribe` (count calls); fire a field-update event → assert NO
  new subscribe/unsubscribe calls; add a session → assert exactly the new
  session's subscriptions are added.
- Unit 2: mock `client.GET` with two deferred resolves; trigger refetch twice;
  resolve in reverse order → assert only the later one is applied; resolve with
  `error` → assert no overwrite.

## Risks
- **Ordering vs ws-lifecycle**: implementing before ws-lifecycle's ref-counted
  teardown would mean the effect's unsubscribe doesn't close sockets — hence the
  feature-level `depends_on`. With teardown in place, the stabilized effect
  re-runs rarely, so teardown churn is minimal.

## Design decisions
- **Stable id-set key + untrack** over `untrack(() => sessions)` alone — keying
  on the derived id string makes the re-run condition explicit (set membership),
  not just suppressing the dependency.
- **Per-session seq guard** over a global one — sessions refetch independently;
  a global counter would cross-cancel unrelated sessions.

## Other agent review

Codex (cross-model, xhigh) reviewed this design. Verdict: approve with two
must-fix clarifications. Confirmed: the `$derived` id-key + `untrack` idiom is
sound (unchanged string key skips reruns); the ws-lifecycle macrotask linger
covers ALL surviving sessions (cleanup + rerun body are synchronous before the
timer fires); `sort()` is on the fresh mapped array; per-session seq guard
correctly drops older overlapping GETs.

**Accepted & applied:**
- **Unit 1**: clarified the acceptance — this is unsub-all/resub-all + linger
  (surviving sockets stay open via synchronous re-subscribe), NOT a delta map;
  noted the `untrack` import.
- **Unit 2**: status-event handlers (`session.finalizing`/`session.ended`) bump
  the per-session `refetchSeq` so an in-flight `commit.arrived` refetch can't
  resolve later and overwrite the ended status; documented the Map as bounded by
  component lifetime with monotonic (non-reset) counters.
