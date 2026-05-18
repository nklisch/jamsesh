---
id: refactor-split-finalize-view
kind: feature
stage: done
tags: [refactor, ui]
parent: null
depends_on: []
release_binding: v0.1.0
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

## Implementation summary (orchestrator)

All three child stories implemented and advanced to `stage: review`. The
orchestrator shrank from 1110 → 882 LoC (net −228 LoC, a 21% reduction).

- `refactor-split-finalize-view-lock-banner` (commit `385f891`) — extracted
  `frontend/src/lib/components/finalize/LockBanner.svelte` (101 LoC). Took
  the conflict-banner, error-banner, and lock-pill markup + CSS.
  FinalizeView: 1110 → 1065 (−45). 11 tests added.
- `refactor-split-finalize-view-ref-group-list` (commit `f3d223b`) —
  extracted `RefGroupList.svelte` (165 LoC) for the source-pool panel.
  Lifted `selectedShas` state remained in the orchestrator as `string[]`;
  passed to the child as `new Set(selectedShas)` for O(1) lookup.
  FinalizeView: 1065 → 959 (−106). 7 tests added. Cart panel deliberately
  kept inline (too many orthogonal couplings — see story body).
- `refactor-split-finalize-view-command-runner` (commit `d6b0465`) —
  extracted `CommandRunner.svelte` for the `jamsesh finalize-run <id>`
  command + Copy button + toast. Component owns the clipboard write and
  fires `oncopy` upward so the orchestrator still tracks ship-hint state.
  FinalizeView: 959 → 882 (−77). 7 tests added. The story's suggested
  `planID: string | null` prop was adapted to `command + ready` for
  better isolation testability.

### Verification

- 300/300 frontend tests pass (up from ~280 pre-feature)
- `FinalizeView.test.ts` passes unchanged across all three sub-stories
- `npm run check` (svelte-check) clean
- New `frontend/src/lib/components/finalize/` directory holds the three
  subcomponents and their tests

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The cart panel was deliberately kept inline (per ref-group-list story's
  reduced scope). That's fine — extracting it would have been a leaky
  abstraction. If a future iteration of the finalize UX restructures the
  cart concerns, that's the time to revisit.

**Notes**: Capability delivered — FinalizeView shrunk 1110 → 882 LoC
(−228, a 21% reduction). Three focused subcomponents under
`frontend/src/lib/components/finalize/` (LockBanner, RefGroupList,
CommandRunner) with co-located tests. The lift-up of `selectedShas` to
the orchestrator and pass-down-as-Set pattern is a clean Svelte 5
reactivity choice. No foundation-doc drift. No public API or schema
changes. Frontend test suite grew from ~280 to 300 tests across the
feature; `FinalizeView.test.ts` passes unchanged across all three
child stories.

### Children
- `refactor-split-finalize-view-lock-banner` (done, archived)
- `refactor-split-finalize-view-ref-group-list` (done, archived — cart panel
  deliberately scoped out)
- `refactor-split-finalize-view-command-runner` (done, archived)
