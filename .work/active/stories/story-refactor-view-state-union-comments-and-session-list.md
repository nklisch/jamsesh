---
id: story-refactor-view-state-union-comments-and-session-list
kind: story
stage: implementing
tags: [ui, refactor]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Convert CommentsTab + SessionList to view-state-union pattern

## Brief

Two Svelte components track load state with a pair of booleans
(`isLoading: $state(true)` + `loadError: $state<string | null>(null)`),
which allows nonsensical state combinations (both true at once) and
forces every consumer block to read two reactive values to decide
which UI branch to render. The project's documented
`view-state-union-machine` pattern (in
`.claude/skills/patterns/view-state-union-machine.md`) explicitly
replaces this shape with a typed `'loading' | 'ready' | 'error'`
union.

Surfaced by a discovery-mode `/agile-workflow:refactor-design` scan.

## Targets

- `frontend/src/lib/components/CommentsTab.svelte` — lines 19-20, 23,
  36, 90-95
- `frontend/src/lib/screens/SessionList.svelte` — lines 22, 43, 53, 164

## Current state (CommentsTab)

```ts
let isLoading = $state(true);
let loadError = $state<string | null>(null);
// ...
async function fetchComments() {
  isLoading = true;
  loadError = null;
  // ...
  if (error)   loadError = '...';
  else if (data) comments = data.items;
  isLoading = false;
}
```

```svelte
{#if isLoading}        <p>Loading…</p>
{:else if loadError}   <p>{loadError}</p>
{:else if comments.length === 0} <p>No comments yet.</p>
{:else}                <CommentGrid {comments} />
{/if}
```

## Target state

```ts
type LoadState =
  | { kind: 'loading' }
  | { kind: 'ready' }
  | { kind: 'error', message: string };

let load = $state<LoadState>({ kind: 'loading' });
// ...
async function fetchComments() {
  load = { kind: 'loading' };
  // ...
  if (error)   load = { kind: 'error', message: '...' };
  else if (data) { comments = data.items; load = { kind: 'ready' }; }
}
```

```svelte
{#if load.kind === 'loading'} ...
{:else if load.kind === 'error'} <p>{load.message}</p>
{:else if comments.length === 0} <p>No comments yet.</p>
{:else} <CommentGrid {comments} />
{/if}
```

Apply the same shape to `SessionList.svelte`.

## Acceptance criteria

- Both components track load state via a single `$state<LoadState>`
  rune (discriminated union).
- Template branches read the discriminant — no double-boolean reads.
- `npm run check` clean.
- `npm run test` passes; existing component tests do not need
  behavior changes (assertion shapes may need updating for the new
  state shape).
- No visible UI change — same loading / error / empty / list branches
  render at the same times.

## Notes

Behavior-preserving — same UI states, same transitions, just a
typed-union representation. Pattern reference:
`.claude/skills/patterns/view-state-union-machine.md`.
