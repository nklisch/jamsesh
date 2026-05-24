---
id: story-refactor-frontend-god-components-session-view-shell
kind: story
stage: implementing
tags: [ui, refactor]
parent: feature-refactor-frontend-god-components
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Decompose SessionViewShell into rune modules + dialog component

## Brief

`frontend/src/lib/screens/SessionViewShell.svelte` is 800 lines. It
orchestrates the entire authenticated session UI: session load, tree
state, bottom panel, comment composer, playground countdown, ref-action
menu, multiple WS subscriptions, and two dialogs (fork + mode-switch).

## Extraction targets

Read the file end-to-end before deciding boundaries. Strong candidates:

1. **`useTreeState.svelte.ts`** — tree-state machine
   (`'tree-collapsed' | 'tree-expanded' | 'tree-wide'`) with localStorage
   persistence keyed by sessionId. Currently lines ~53-76 of the file.

2. **`usePlaygroundCountdown.svelte.ts`** — playground-mode state
   (`isPlayground`, `playgroundHardCapAt`, `playgroundIdleTimeoutAt`,
   `playgroundLastActivityAt`, `idleRemainingMs`, `hardCapRemainingMs`)
   plus the WS handlers for `playground.activity_reset` and
   `session.destroyed`. Currently lines ~104-130.

3. **`useRefActions.svelte.ts`** — ref-action menu + dialog state
   (`activeMenuRef`, `activeDialog`, `activeDialogRef`,
   `activeDialogRefMode`) plus the open/close handlers (`handleRefAction`,
   `handleMenuAction`, `closeDialog`). Currently lines ~129-151.

4. **`useCommentComposer.svelte.ts`** — comment composer state
   (`composerOpen`, `composerRange`) + the range-select handler.
   Currently lines ~100-127.

Use `wrapper-object-rune-store`. Place modules in
`frontend/src/lib/session/` (or wherever the codebase already groups
session-screen helpers).

The shell template stays mostly intact — it reads from the rune-module
facades and continues to render `Chrome`, `TreeDag`, `ActivityFeed`,
`CommentsTab`, `ArtifactPane`, `CommentComposer`, `RefActionsMenu`,
`ForkDialog`, `ModeSwitchDialog`, `WsStatusBanner`, `PlaygroundChip`,
`CountdownBadge`, `DestructionWarningBanner` exactly as today.

## Acceptance criteria

- [ ] `SessionViewShell.svelte` LoC ≤ 350.
- [ ] At least 3 of the 4 candidate rune modules above are extracted.
- [ ] Each new module follows `wrapper-object-rune-store`.
- [ ] No visible UI change — same tree-state cycling, same playground
      countdown behavior, same dialog open/close semantics, same
      composer activation flow.
- [ ] `npm run check` clean.
- [ ] `npm run test` passes (this screen has the most existing test
      coverage; expect assertions to need light adjustment for new
      module boundaries).
- [ ] `npm run build` clean.

## Risk

**Medium.** SessionViewShell is the central authenticated screen and
hosts multiple WS subscriptions whose ordering matters. Reactive-graph
breakage is the main risk. Mitigation: preserve `$effect` dependency
shapes inside each rune-module facade.

## Rollback

`git revert` the commit.

## Out of scope

- The two inline WS event-type annotations (`PlaygroundActivityResetEvent`,
  `SessionDestroyedEvent`) — those are blocked on
  `idea-playground-ws-event-types-missing-from-openapi`. Leave them in
  place; they move with `usePlaygroundCountdown` if you extract it.
