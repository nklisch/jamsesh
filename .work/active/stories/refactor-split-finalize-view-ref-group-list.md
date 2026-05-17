---
id: refactor-split-finalize-view-ref-group-list
kind: story
stage: implementing
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
