---
id: refactor-split-finalize-view
kind: feature
stage: implementing
tags: [refactor, ui]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Refactor — Split `FinalizeView.svelte`

## Why

`frontend/src/lib/screens/FinalizeView.svelte` is 1110 lines, 404 of which
are inside `<script>`. It owns 8 state runes and 21 in-component functions
spanning concerns that are independently testable:

- **Lock state machine** — fetch lock, detect conflict, surface "you're not
  the caller" banner, poll until acquired
- **Plan fetching + display** — load the finalize plan once lock is held,
  render the cherry-pick command
- **Ref grouping** — categorize refs (sync vs isolated, included vs excluded)
  for the curation tree
- **Commit cart** — track selected SHAs, derive co-author chips, run the
  squash message editor
- **Command runner** — render the one-line `jamsesh finalize-run` and copy-
  to-clipboard / open-in-terminal affordances

The screen is the most complex in the SPA. Splitting it improves
maintainability, lets each piece be tested without standing up the whole
view, and makes future curation-UX iteration cheaper.

## Constraint

The split must preserve the current `FinalizeView` URL and external
contract — `App.svelte`'s router resolves `/sessions/:sessionID/finalize`
to the existing component. Internal restructuring only.

## Target shape

`FinalizeView.svelte` shrinks to ~250-350 lines and becomes an orchestrator
that wires three new subcomponents and owns only the cross-cutting state
(session ID, lock status, plan):

```
frontend/src/lib/screens/FinalizeView.svelte           # ~250 LoC, orchestrator
frontend/src/lib/components/finalize/LockBanner.svelte # lock state + banner
frontend/src/lib/components/finalize/RefGroupList.svelte # ref tree + selection
frontend/src/lib/components/finalize/CommandRunner.svelte # command + copy/run
```

`SquashMessageEditor.svelte` and `CoAuthorChipRow.svelte` already exist —
the orchestrator continues to use them directly.

State that crosses subcomponents (`selectedShas`, `lockStatus`, `plan`)
lives in the orchestrator and is passed down via props. The orchestrator
also owns the API calls.

## Acceptance criteria for the feature

- [ ] `FinalizeView.svelte` is ≤ 400 LoC
- [ ] Three new subcomponents exist under
      `frontend/src/lib/components/finalize/` with co-located tests
- [ ] `FinalizeView.test.ts` passes unchanged (the orchestrator's external
      contract is unchanged)
- [ ] Each subcomponent has its own test file with at least one rendering
      test and one interaction test
- [ ] Dev-server walkthrough of the finalize flow shows identical behavior
      to pre-refactor

## Risk

MEDIUM. The screen is heavy and has subtle state interactions (lock
polling races, plan freshness). Mitigations:

- Split one subcomponent at a time so regressions are easy to bisect.
- Run `FinalizeView.test.ts` after every story merge.
- A11y: the modal/banner interactions must keep their existing ARIA roles.

## Implementation order

1. `refactor-split-finalize-view-lock-banner` — extract LockBanner (lowest
   risk, smallest blast radius)
2. `refactor-split-finalize-view-ref-group-list` — extract RefGroupList
3. `refactor-split-finalize-view-command-runner` — extract CommandRunner

Children are listed in implementation order, but the depends_on chain is
linear — each story depends on the previous so the orchestrator's prop
surface stabilizes incrementally.

## Design decision (autopilot)

Stage advanced `drafting → implementing` directly without invoking
`refactor-design` per-feature mode. Feature was emitted by discovery
mode with full body, target shape, acceptance, and chained child stories.
Per-feature mode would re-design content already present in the children.
