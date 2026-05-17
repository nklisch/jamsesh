---
id: refactor-split-finalize-view-command-runner
kind: story
stage: implementing
tags: [refactor, ui]
parent: refactor-split-finalize-view
depends_on: [refactor-split-finalize-view-ref-group-list]
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Finalize split — Extract `<CommandRunner>`

## Files

- New: `frontend/src/lib/components/finalize/CommandRunner.svelte`
- New: `frontend/src/lib/components/finalize/CommandRunner.test.ts`
- Modify: `frontend/src/lib/screens/FinalizeView.svelte`

## What moves

From `FinalizeView.svelte`:

- The `jamsesh finalize-run <plan-id>` command rendering
- Copy-to-clipboard button and its success-feedback affordance
- Any "command is ready / waiting for plan / failed to fetch plan"
  status display

What stays in the orchestrator:

- The plan fetch call (orchestrator owns plan state; subcomponent
  receives the plan ID as a prop)

## Props shape

```ts
type Props = {
  planID: string | null;  // null when plan is still loading or unavailable
  errorMessage?: string;  // surfaced when plan fetch failed
};
```

## Acceptance

- [ ] `CommandRunner.svelte` renders the one-line command with copy
      affordance when `planID` is non-null
- [ ] When `planID` is null, renders a loading or error state
- [ ] `CommandRunner.test.ts` covers: loading, ready (copy works),
      error states
- [ ] `FinalizeView.svelte` orchestrator shrinks below 400 LoC after this
      story merges
- [ ] `FinalizeView.test.ts` passes unchanged

## After this story

The parent feature `refactor-split-finalize-view` can be advanced to
`stage: review`.

## Risk

LOW.

## Rollback

`git revert` the commit.
