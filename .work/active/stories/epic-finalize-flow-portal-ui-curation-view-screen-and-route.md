---
id: epic-finalize-flow-portal-ui-curation-view-screen-and-route
kind: story
stage: implementing
tags: [ui]
parent: epic-finalize-flow-portal-ui-curation-view
depends_on: [epic-finalize-flow-plan-generation-locks-schema-and-rest, epic-finalize-flow-plan-generation-plan-fetch-and-script, epic-finalize-flow-plan-generation-fetch-token-and-mark-shipped]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Finalize curation view — screen, route, and cart flow

## Scope

Land the FinalizeView screen at the new `finalize` route. Owns the
full curation-view lifecycle: lock acquisition, source-pool + cart
layout, mode bar, PATCH debouncing, plan re-fetch, WebSocket
reactions, the lock-conflict banner, the "Run locally" copy CTA,
and the "Mark as shipped" transition.

Squash-mode sub-components (`SquashMessageEditor.svelte` +
`CoAuthorChipRow.svelte`) are imported but may render as
placeholders (no-op components emitting empty markup) — story 2
implements them. The screen must still pass `commit_message` and
the co-author list through to the placeholder props so swapping in
the real implementation is a one-line lift.

## Units delivered

- `frontend/src/lib/screens/FinalizeView.svelte` — the screen
  (component tree, $state, debounced PATCH, refetchPlan, WS
  subscriptions, lock-conflict banner, source-pool, cart, copy
  CTA, mark-shipped CTA)
- `frontend/src/lib/screens/FinalizeView.test.ts` — vitest
  coverage per the parent feature's test plan, items 1-13
- `frontend/src/lib/router.svelte.ts` — add the `finalize` route
  ahead of `session-view`
- `frontend/src/App.svelte` — render `<FinalizeView/>` on
  `current.name === 'finalize'`
- `frontend/src/lib/screens/SessionViewShell.svelte` — wire the
  existing `<button class="header-btn">Finalize</button>` to
  `navigate(\`/orgs/\${orgId}/sessions/\${sessionId}/finalize\`)`
- Empty placeholder modules for the sub-components (story 2
  fills these in):
  - `frontend/src/lib/components/SquashMessageEditor.svelte`
    (renders a textarea bound to the `message` prop, fires
    `onmessagechange` on input — minimum viable so the parent
    test exercises the data flow)
  - `frontend/src/lib/components/CoAuthorChipRow.svelte`
    (renders nothing; story 2 implements)

## Acceptance criteria

- New route pattern matches `/orgs/<org>/sessions/<sid>/finalize`
  and renders FinalizeView.
- Mount sequence: POST lock → if 409 render banner; else load
  plan, populate state, subscribe to WS events.
- Curation mutations (toggle, reorder, target-branch input, mode
  flip, commit-message edit) update local state optimistically
  and schedule a single trailing-edge PATCH at 300ms.
- Stale PATCH responses do not refresh the plan when a newer
  PATCH is in flight.
- WS reaction matrix per feature design (lock-acquired,
  lock-released, session.ended, mode.changed) drives the right
  state transitions.
- "Run locally" copies `jamsesh finalize-run <plan_id>` to
  clipboard and shows a 1.5s toast.
- "Mark as shipped" POSTs the endpoint and on success transitions
  the page into the ended state.
- Override on the lock-conflict banner reloads the page with a
  fresh override-acquired lock.
- All test-plan items 1-13 from the parent feature design pass.
- `cd frontend && pnpm test` green; `cd frontend && pnpm check`
  clean.

## Notes

- `available_refs` shape on the lock response is consumed via the
  generated `FinalizeLock`/`PlanResponse` types from
  `$lib/api/types.gen`. If `plan-generation` lands a slightly
  different field name (e.g. `groups` or `pool`), adjust the
  client-side `RefGroup` derivation to match — keep the cart
  semantics identical.
- Drag-to-reorder is intentionally out of scope for v1.
  Up/down (▲/▼) buttons satisfy reorder + accessibility per the
  parent feature design.
