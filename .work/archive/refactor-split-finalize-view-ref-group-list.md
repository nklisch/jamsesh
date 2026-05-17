---
id: refactor-split-finalize-view-ref-group-list
kind: story
stage: done
tags: [refactor, ui]
parent: refactor-split-finalize-view
depends_on: [refactor-split-finalize-view-lock-banner]
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Finalize split — Extract `<RefGroupList>`

## Files

- New: `frontend/src/lib/components/finalize/RefGroupList.svelte`
- New: `frontend/src/lib/components/finalize/RefGroupList.test.ts`
- Modify: `frontend/src/lib/screens/FinalizeView.svelte`

## What moves

From `FinalizeView.svelte`:

- Ref grouping logic (sync vs isolated, included vs excluded)
- Per-commit selection markup (checkboxes, commit metadata rendering)
- Per-ref expand/collapse state
- The selection-cart visualization
- Related CSS

What stays in the orchestrator:

- The `selectedShas` state itself (lifted up — the SquashMessageEditor and
  CommandRunner both depend on it)
- The plan-fetching call

## Props shape

```ts
type Props = {
  refs: RefGroup[];                 // grouped sync/isolated refs with commits
  selected: Set<string>;            // SHAs currently selected
  onToggle: (sha: string) => void;  // emit selection changes upward
};
```

## Acceptance

- [ ] `RefGroupList.svelte` renders sync and isolated groups with their
      commits, checkboxes, and expand/collapse affordances
- [ ] `RefGroupList.test.ts` covers: empty state, sync-only, isolated-only,
      mixed; toggling a checkbox fires `onToggle` with the right SHA
- [ ] `FinalizeView.svelte` orchestrator owns `selectedShas` as a `Set<string>`
      state rune and passes `selected` + `onToggle` down
- [ ] `FinalizeView.test.ts` passes unchanged

## Risk

MEDIUM. The selection state is shared across multiple subcomponents — the
prop lift must be careful to preserve reactivity. Use a `Set<string>`
state rune in the orchestrator and a plain `onToggle` callback rather
than a `bind:`-mediated cross-component sync.

## Rollback

`git revert` the commit; the orchestrator's prior inline implementation
is restored.

## Implementation notes

- `selectedShas` stayed as `string[]` in the orchestrator (option 1 from the story). The
  orchestrator passes `selected={new Set(selectedShas)}` to `RefGroupList`, keeping the
  array canonical for the PATCH body (`selected_commit_shas: selectedShas`).
- **Reduced scope**: cart panel stayed inline in `FinalizeView`. The cart depends on
  `targetBranch`, `commitMessage`, `mode`, `plan`, `distinctAuthors`, `canRun`, `runCommand`,
  `markShipped`, `copyRunCommand`, and `setTargetBranch` — too many orthogonal orchestrator
  concerns to cleanly extract. Only the source-pool panel was moved.
- **LoC delta**: FinalizeView 1065 → 959 lines (−106). New component: 165 lines.
- **onAddAll callback**: `RefGroupList` also accepts an optional `onAddAll?: (group: RefGroup) => void`
  prop, wired to `addAllInGroup` in the orchestrator.
- **Test count**: 7 new tests in `RefGroupList.test.ts`; total suite 293 (was 286).

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: 165-LoC component cleanly extracts the source-pool panel.
The deliberate scope reduction (cart panel stays inline due to orthogonal
couplings — `targetBranch`, `commitMessage`, `mode`, `plan`,
`distinctAuthors`, `canRun`, `runCommand`, `markShipped`) is the right
call — forcing extraction would have produced a leaky abstraction. The
`Set<string>` reactivity pattern (orchestrator owns canonical `string[]`,
passes `new Set(selectedShas)` for O(1) lookup, callbacks fire `onToggle`
upward) is correct for Svelte 5 — avoids `bind:` cross-component coupling.
FinalizeView 1065 → 959 (−106 LoC). 7 new tests cover the 7 spec'd
scenarios. FinalizeView's 13 existing tests pass unchanged.
