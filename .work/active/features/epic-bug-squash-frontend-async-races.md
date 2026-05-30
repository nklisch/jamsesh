---
id: epic-bug-squash-frontend-async-races
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

# Frontend async & reactive-state race hardening

## Brief

The SPA has a cluster of async/reactive-state correctness bugs across screens
and components. The bug-scan found seven: an ArtifactPane `$effect` fetch with
no stale-response guard (wrong file's content renders, High), module-singleton
finalize stores holding per-session state across mounts (wrong-lock release /
cross-session bleed, High), a `requestMagicLink` raw fetch with no try/catch
(network failure silently hangs the form), a ForkDialog refs fetch that always
fails because `orgIdFromRef` returns `""` (fork targets the wrong tip), an
event-driven refetch with no sequence guard (stale-overwriting-fresh), a
SessionList subscription `$effect` that re-subscribes on every event (churn),
and a CountdownBadge per-tick parent write.

This feature delivers race-hardened frontend async: request-sequence/abort
guards on effect-driven fetches, per-instance isolation for the finalize stores,
consistent error handling on raw fetches, a correctly-scoped ForkDialog request,
and a stable subscription effect. It covers these screen/component/store defects
only; it does NOT redesign the SPA routing or the rune-store conventions.

It depends on `epic-bug-squash-frontend-ws-lifecycle` because the SessionList
subscription/refetch fixes build on the corrected `subscribe`/`close` contract
from that feature.

## Epic context
- Parent epic: `epic-bug-squash`
- Position in epic: consumer of `epic-bug-squash-frontend-ws-lifecycle` (the WS
  manager's corrected lifecycle). Largest feature (7 stories) — feature-design
  may sub-group by screen.

## Foundation references
- `docs/SPEC.md` — Svelte 5 SPA, openapi-fetch typed client
- Patterns: `per-instance-factory-rune-store`, `wrapper-object-rune-store`,
  `openapi-fetch-result-branch`, `view-state-union-machine`

## Child stories (pre-existing, from bug-scan — re-parented here)
- `bug-squash-artifactpane-stale-fetch-overwrite` — High, async — `frontend/src/lib/components/ArtifactPane.svelte:25`
- `bug-squash-finalize-stores-module-singletons` — High, async — `frontend/src/lib/finalize/useFinalizeLock.svelte.ts:18`
- `bug-squash-magic-link-fetch-no-trycatch` — Medium, async — `frontend/src/lib/screens/Login.svelte:110`
- `bug-squash-forkdialog-empty-org-refs-fetch` — Medium, async — `frontend/src/lib/components/ForkDialog.svelte:48`
- `bug-squash-ws-refetch-stale-overwrite` — Medium, async — `frontend/src/lib/screens/SessionList.svelte:78`
- `bug-squash-sessionlist-resubscribe-churn` — Medium, state — `frontend/src/lib/screens/SessionList.svelte:68`
- `bug-squash-countdownbadge-per-tick-write` — Low, async — `frontend/src/lib/components/CountdownBadge.svelte:50`

<!-- feature-design fills in the per-fetch sequence-guard idiom, the finalize
store factory conversion, and the vitest approach. -->
