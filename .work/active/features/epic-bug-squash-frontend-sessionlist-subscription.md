---
id: epic-bug-squash-frontend-sessionlist-subscription
kind: feature
stage: drafting
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

<!-- feature-design fills in the stable-id-set effect key and the per-fetch
sequence-guard idiom (mirroring useFinalizePlan._patchSeq), building on the
ws-lifecycle subscribe/close contract. -->
