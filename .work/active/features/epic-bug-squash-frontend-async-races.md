---
id: epic-bug-squash-frontend-async-races
kind: feature
stage: review
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

**Implementation Notes**: production importer is ONLY `FinalizeView` (codex
verified); the four finalize **test files import the singletons directly** and
must convert to `createFinalizeX()` + isolation assertions. Keep each facade's
method bodies byte-identical; only the wrapper (module var → closure var)
changes. Also update the `per-instance-factory-rune-store` / `wrapper-object-rune-store`
pattern doc, which currently says finalize stores stay module-level (now stale).

**Codex must-fix — late server side effect**: factory isolation fixes stale
*state* writes, but `FinalizeView.onMount` does `await acquireLock();
startSubscriptions();` — a destroy during that await still (a) acquires a real
server lock after `onDestroy`, and (b) starts subscriptions post-teardown. Add an
`alive` generation flag (set false in `onDestroy`); after the `await`, if
`!alive` → `void finalizeLock.release(orgId, sessionId)` for the late-acquired
lock and SKIP `startSubscriptions()`:

```ts
let alive = true;
onMount(() => { void (async () => {
  await finalizeLock.acquire(orgId, sessionId, opts);
  if (!alive) { void finalizeLock.release(orgId, sessionId); return; } // unmounted during acquire
  startSubscriptions();
})(); });
onDestroy(() => { alive = false; /* ...existing teardown... */ });
```

**Acceptance Criteria**:
- [ ] Two sequential FinalizeView mounts (A unmount → B mount) do not share
      lock/plan/curation/execution state; B starts clean.
- [ ] A destroy DURING `acquireLock()` releases the late-acquired server lock and
      does NOT call `startSubscriptions()` (alive guard).
- [ ] `onDestroy` releases only the lock whose `lock_id` the instance holds.
- [ ] The 4 finalize test files use `createFinalizeX()` and assert cross-instance
      isolation.

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

**Implementation Notes**: `SessionViewShell` renders `<ForkDialog>` and has
`orgId` in scope (codex verified) — thread it as a prop. **Codex must-fix**: also
GATE the fork itself — treat "source ref not found in the refs response" the same
as a refs-fetch failure and STOP before issuing the fork (the `/mcp` fork call),
surfacing an error. Otherwise the dialog can still fork without a
`target_commit_sha` even after the URL is fixed.

**Acceptance Criteria**:
- [ ] The refs fetch uses the real org id and resolves the selected source ref's
      tip SHA; a failed fetch OR an unresolved ref surfaces an error and the fork
      is NOT issued (no silent wrong-tip fork).

### Unit 5: CountdownBadge per-tick callback guard (Low)
**File**: `frontend/src/lib/components/CountdownBadge.svelte`
**Story**: `bug-squash-countdownbadge-per-tick-write` (Low)

The `$effect` calls `onremainingupdate(...)` every tick (once/sec), driving a
cross-component write + downstream re-derive. **Codex must-fix**: a
"second-rounded changed-value guard" is a NO-OP here — the badge ticks once per
second, so every tick already produces a new rounded value. The real fix is to
**relocate the remaining-time derivation to the parent**, which already holds
`hardCapAt`/`idleTimeoutAt`: the parent derives `idleRemainingMs`/`hardCapRemainingMs`
from its own `now`/timestamps (or a shared countdown store), and CountdownBadge
becomes display-only — dropping the per-tick child→parent callback entirely.

**Acceptance Criteria**:
- [ ] The parent derives remaining-time from its own timestamps; CountdownBadge
      no longer pushes a per-tick `onremainingupdate` into the parent (the
      cross-component per-second write is gone).

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
- **CountdownBadge**: relocate the remaining-time math to the parent (codex: the
  changed-value guard is a no-op at 1 tick/sec) — removes the per-tick
  cross-component write at the source.

## Other agent review

Codex (cross-model, xhigh) reviewed this design. Verdict: approve the direction;
fix Unit 1 lifecycle + Unit 5 acceptance before implementation. Confirmed:
the factory-rune pattern is valid Svelte 5 here (`createTreeState`,
`createRefActions`, `createPlaygroundCountdown`, `createCommentComposer`,
`createNewSessionForm` already use it); production importer of the finalize
singletons is ONLY FinalizeView; ArtifactPane AbortController is correct;
`SessionViewShell` has `orgId` to pass to ForkDialog; `$derived` usage is
confined to `useFinalizeCuration`.

**Accepted & applied:**
- **Unit 1 (late server side effect)**: added an `alive` generation guard —
  after `await acquireLock()`, if unmounted, release the late-acquired server
  lock and skip `startSubscriptions()`. Plus: convert the 4 finalize TEST files
  to factories; update the pattern doc (it currently says finalize stays
  module-level).
- **Unit 4 (fork gating)**: treat an unresolved source ref the same as a
  refs-fetch failure — STOP before issuing the fork (no `target_commit_sha`),
  not just fix the URL.
- **Unit 5 (no-op guard)**: the changed-value guard doesn't help at 1 tick/sec;
  relocate the remaining-time math to the parent and make the badge display-only.

## Implementation summary

All 5 child stories implemented and advanced to `stage: review` (per-story `implement: bug-squash-*` commits). Each landed a failing-first regression test; the codex feature-gate findings (see `## Other agent review`) were applied during design and honored in implementation. Verification at the orchestrator level: `go build ./...` + `go vet` clean; backend `-race`/package tests and frontend `vitest` (764 passing) + `svelte-check` green; `sqlc generate` matches spec.

## Final-gate fix

**Finding 3 (BLOCKING): ForkDialog `/mcp` fork POST missing bearer auth.**
`ForkDialog.svelte` line 88: the fetch to `/mcp` had no `Authorization` header.
The MCP endpoint requires bearer auth (`internal/portal/mcpendpoint/handler.go:94`).
Fix: import `auth` from `$lib/auth.svelte`; read `auth.token` before the fetch;
set `Authorization: Bearer ${token}` on the request headers (same pattern as
`ArtifactPane.svelte`).

Added test `'fork POST to /mcp carries Authorization: Bearer header'` in
`ForkDialog.test.ts`: mocks `$lib/auth.svelte` with a known token, resolves refs,
clicks Fork, asserts `fetch('/mcp')` was called with `Authorization: Bearer
test-bearer-token-abc123`.
