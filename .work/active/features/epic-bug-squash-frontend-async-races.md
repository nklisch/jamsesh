---
id: epic-bug-squash-frontend-async-races
kind: feature
stage: implementing
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

## Architectural choice

**Local fixes per screen/component, plus a per-instance-factory conversion for
the finalize stores.** Four are localized (abort/try-catch/prop/tick-guard); the
finalize-store fix follows the existing `per-instance-factory-rune-store` pattern.
All 5 stories are independent files — parallelizable.

## Implementation Units

### Unit 1: Finalize stores → per-instance factories (High)
**Files**: `frontend/src/lib/finalize/useFinalize{Lock,Plan,Curation,Execution}.svelte.ts`,
`frontend/src/lib/screens/FinalizeView.svelte`
**Story**: `bug-squash-finalize-stores-module-singletons` (High)

The 4 stores are module-level singletons (`_`-prefixed `$state` + exported
facade object), so per-session state is shared and an in-flight async write from
a torn-down instance lands in the shared singleton. Convert each to a factory
matching the project's `per-instance-factory-rune-store` pattern:

```ts
// useFinalizeLock.svelte.ts
export function createFinalizeLock() {
  let _lock = $state<LockStatus | null>(null);
  let _lockConflict = $state(null); /* ... */
  return {
    get status() { return _lock; },
    async acquire(orgId, sessionId, opts) { /* unchanged body */ },
    async release(orgId, sessionId) { /* unchanged */ },
    /* ...same facade... */
  };
}
```

```svelte
<!-- FinalizeView.svelte <script> -->
import { createFinalizeLock } from '$lib/finalize/useFinalizeLock.svelte';
// ...
const finalizeLock = createFinalizeLock();      // per mount
const finalizePlan = createFinalizePlan();
const finalizeCuration = createFinalizeCuration();
const finalizeExecution = createFinalizeExecution();
```

Every existing `finalizeLock.foo` reference in FinalizeView is unchanged — only
the 4 imports become `createX` + one instantiation each. A fresh instance per
mount means: no cross-session bleed, the `onMount` `reset()` calls become
redundant (drop them — the factory starts clean), and `onDestroy` reads ITS
instance's `status`/`lock_id` so it can only release the lock it owns. Stale
async writes after unmount land in the dead (GC'd) instance, harmless.

**Implementation Notes**: `grep` every importer of the 4 singletons — if ONLY
FinalizeView imports them, the conversion is contained; otherwise each importer
instantiates its own (and shares via props if they must coordinate). Keep each
facade's method bodies byte-identical; only the wrapper (module var → closure
var) changes. Confirm no consumer relied on the module-level singleton identity
(codex epic-gate caveat).

**Acceptance Criteria**:
- [ ] Two sequential FinalizeView mounts (session A unmount → session B mount) do
      not share lock/plan/curation/execution state; B starts clean.
- [ ] An `acquire`/`refetch` await that resolves after `onDestroy` does not
      mutate a live instance (it writes to the orphaned one).
- [ ] `onDestroy` releases only the lock whose `lock_id` the instance holds.

### Unit 2: ArtifactPane stale-fetch abort guard (High)
**File**: `frontend/src/lib/components/ArtifactPane.svelte`
**Story**: `bug-squash-artifactpane-stale-fetch-overwrite` (High)

The `$effect` fetch has no cancellation, so a slow response for file A overwrites
the current file B. (orgId IS already a prop — no prop change needed.) Add an
`AbortController` scoped to each effect run:

```ts
$effect(() => {
  if (!selectedSha || !selectedPath) { /* reset */ return; }
  loading = true; loadError = null;
  const controller = new AbortController();
  fetch(url, { headers, signal: controller.signal })
    .then((r) => { if (!r.ok) throw new Error(`HTTP ${r.status}`); return r.json(); })
    .then((data) => { if (controller.signal.aborted) return; content = data.content; isBinary = data.is_binary; mime = data.mime; })
    .catch((e) => { if (controller.signal.aborted) return; loadError = ...; })
    .finally(() => { if (!controller.signal.aborted) loading = false; });
  return () => controller.abort(); // effect cleanup aborts the prior fetch
});
```

**Acceptance Criteria**:
- [ ] Selecting file A then file B before A resolves leaves B's content rendered
      (A's late response is aborted/ignored, not written).

### Unit 3: Login requestMagicLink try/catch (Medium)
**File**: `frontend/src/lib/screens/Login.svelte`
**Story**: `bug-squash-magic-link-fetch-no-trycatch` (Medium)

Wrap the raw `fetch`/`await` in try/catch (matching the sibling
`signInWithGitHub`) so a transport failure sets `mode='magic-link-error'` +
`errorMsg` instead of floating an unhandled rejection and hanging the form.

**Acceptance Criteria**:
- [ ] A rejected `fetch` (network failure) lands in `magic-link-error` with a
      message; no unhandled rejection.

### Unit 4: ForkDialog org-scoped refs fetch (Medium)
**File**: `frontend/src/lib/components/ForkDialog.svelte` (+ parent instantiation)
**Story**: `bug-squash-forkdialog-empty-org-refs-fetch` (Medium)

`orgIdFromRef` returns `""`, so the refs request hits `/api/orgs//...` and 404s,
silently skipped — the fork targets the wrong tip. Add an `orgId` prop, thread it
from the parent (the session view already has it), use it in the refs URL (prefer
the typed `client.GET('/api/orgs/{orgID}/sessions/{sessionID}/refs')`), and treat
a failed refs fetch as a surfaced error rather than a silent skip. Delete the
dead `orgIdFromRef`.

**Implementation Notes**: verify the parent (SessionView/SessionViewShell)
instantiates `<ForkDialog>` and can pass its `orgId`. If the parent lacks it in
scope, thread it down.

**Acceptance Criteria**:
- [ ] The refs fetch uses the real org id and resolves the selected source ref's
      tip SHA; a failed fetch surfaces an error (no silent wrong-tip fork).

### Unit 5: CountdownBadge per-tick callback guard (Low)
**File**: `frontend/src/lib/components/CountdownBadge.svelte`
**Story**: `bug-squash-countdownbadge-per-tick-write` (Low)

The `$effect` calls `onremainingupdate(...)` every tick (once/sec), driving a
cross-component write + downstream re-derive. Guard it behind a changed-value
check so the parent is only notified when the second-rounded remaining actually
changes (or move the remaining-time math into the parent, which already holds
`hardCapAt`/`idleTimeoutAt`).

**Acceptance Criteria**:
- [ ] `onremainingupdate` fires only when the rounded remaining changes, not on
      every internal `now` tick.

## Implementation Order
All 5 independent (distinct files) — parallelizable. Unit 1 is the largest
(factory conversion + FinalizeView wiring); the rest are small localized edits.

## Testing (vitest + jsdom, existing test patterns)
- Unit 1: instantiate two `createFinalizeLock()` → assert isolated state; an
  await resolving post-`reset`/teardown doesn't touch a new instance.
- Unit 2: mock fetch with a deferred resolve; change `selectedPath`; resolve the
  stale promise → assert content NOT overwritten.
- Unit 3: mock fetch to reject → assert `mode==='magic-link-error'`.
- Unit 4: assert the refs request URL contains the real org id; failure surfaces.
- Unit 5: advance the interval; assert `onremainingupdate` call count equals
  distinct rounded-second values, not tick count.

## Risks
- **Unit 1 importer sweep**: if a non-FinalizeView module imports a finalize
  singleton, the factory conversion must thread the instance to it (props) — the
  grep in Unit 1 notes catches this before implementation.
- **Unit 4 parent prop**: if the parent doesn't have `orgId` in scope, a small
  thread-down is needed — verified during implementation.

## Design decisions
- **Finalize: per-instance factories** (per the codex epic-gate caveat and the
  `per-instance-factory-rune-store` pattern) over an instance-token guard —
  isolation fully resolves both cross-session bleed and stale-write hazards.
- **ArtifactPane: AbortController** over a request-id counter — also cancels the
  in-flight network request, not just the state write.
- **CountdownBadge**: changed-value guard (minimal) over relocating the math —
  smallest fix for a Low.

## Other agent review

_Codex (xhigh) feature peer-review gate pending._
