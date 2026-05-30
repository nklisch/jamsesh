---
id: epic-bug-squash-frontend-async-races
kind: feature
stage: drafting
tags: [bug, ui]
parent: epic-bug-squash
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Frontend async & reactive-state race hardening

## Brief

This feature fixes a cluster of async/reactive-state correctness bugs across SPA
screens and components that are **independent of the WebSocket manager**. The
bug-scan found five: an ArtifactPane `$effect` fetch with no stale-response guard
(wrong file's content renders, High), module-singleton finalize stores holding
per-session state across mounts (wrong-lock release / cross-session bleed, High),
a `requestMagicLink` raw fetch with no try/catch (network failure silently hangs
the form), a ForkDialog refs fetch that always fails because `orgIdFromRef`
returns `""` (fork targets the wrong tip), and a CountdownBadge per-tick parent
write.

This feature delivers race-hardened frontend async: request-sequence/abort
guards on effect-driven fetches, per-instance isolation for the finalize stores,
consistent error handling on raw fetches, and a correctly-scoped ForkDialog
request. It covers these screen/component/store defects only; it does NOT
redesign the SPA routing or the rune-store conventions.

It is **independent** (no dependency on the WS-lifecycle feature) — the
SessionList subscription/refetch fixes that DID depend on the WS contract were
split out into `epic-bug-squash-frontend-sessionlist-subscription` (per the
codex decomposition gate), so the two High fixes here are not blocked.

## Epic context
- Parent epic: `epic-bug-squash`
- Position in epic: independent frontend feature (5 stories incl. 2 Highs). The
  WS-coupled SessionList work lives in the sibling
  `epic-bug-squash-frontend-sessionlist-subscription` feature.

## Foundation references
- `docs/SPEC.md` — Svelte 5 SPA, openapi-fetch typed client
- Patterns: `per-instance-factory-rune-store`, `wrapper-object-rune-store`,
  `openapi-fetch-result-branch`, `view-state-union-machine`

## Design caveats (from codex decomposition gate — feature-design must honor)
- **finalize-stores fix**: convert the module-level finalize stores to the
  `per-instance-factory-rune-store` pattern (`createFinalize*()` closure facades)
  for per-mount isolation, reconciling with the project's rune-store conventions
  (`wrapper-object-rune-store` is for genuinely shared singletons). Confirm no
  consumer relies on the module-level singleton identity before converting; the
  existing `FinalizeView.onMount` reset is NOT sufficient isolation under
  overlapping A→B mount/unmount.

## Child stories (pre-existing, from bug-scan — re-parented here)
- `bug-squash-artifactpane-stale-fetch-overwrite` — High, async — `frontend/src/lib/components/ArtifactPane.svelte:25`
- `bug-squash-finalize-stores-module-singletons` — High, async — `frontend/src/lib/finalize/useFinalizeLock.svelte.ts:18`
- `bug-squash-magic-link-fetch-no-trycatch` — Medium, async — `frontend/src/lib/screens/Login.svelte:110`
- `bug-squash-forkdialog-empty-org-refs-fetch` — Medium, async — `frontend/src/lib/components/ForkDialog.svelte:48`
- `bug-squash-countdownbadge-per-tick-write` — Low, async — `frontend/src/lib/components/CountdownBadge.svelte:50`

<!-- feature-design fills in the per-fetch sequence-guard idiom, the finalize
store factory conversion, and the vitest approach. -->
