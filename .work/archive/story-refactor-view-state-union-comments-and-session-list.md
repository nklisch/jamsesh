---
id: story-refactor-view-state-union-comments-and-session-list
kind: story
stage: done
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

## Implementation notes

**Union form chosen:** String-literal union (`type LoadState = 'loading' | 'ready' | 'error'`)
with a sibling `let loadError = $state('')` rune — matching the canonical pattern file
form. The tagged-object form from the story body was not needed; the pattern explicitly
prefers the simpler string-literal shape when the error case needs only a single string
payload that can live in a sibling rune.

**CommentsTab.svelte**
- Replaced `isLoading: $state(true)` + `loadError: $state<string | null>(null)` with
  `loadState: $state<LoadState>('loading')` + `loadError: $state('')`.
- `fetchComments()` transitions: sets `loadState = 'loading'` at start, then either
  `loadState = 'error'` (with `loadError = '...'`) or `loadState = 'ready'`.
- Template: `{#if loadState === 'loading'} ... {:else if loadState === 'error'} ...`
- No reactive `$derived` read `isLoading`/`loadError` in isolation — no flaw discovered.

**SessionList.svelte**
- Same shape: `loadState: $state<LoadState>('loading')` + `loadError: $state('')`.
- `loadSessions()` transitions mirror CommentsTab.
- Template at line ~171: same `loadState === 'loading'` / `loadState === 'error'` branch.

**Verification**
- `npm run check`: 0 errors, 2 pre-existing warnings (unrelated).
- `npm run test`: 624/624 passed (50 test files), no test modifications needed — tests
  assert rendered text / DOM state, not internal rune names.
- `npm run build`: clean production build, 829ms.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Impossible-case difference: if openapi-fetch ever returned both `data` and `error` as undefined (the `openapi-fetch-result-branch` pattern guarantees one is set), the new code stays at `loadState = 'loading'` whereas the old code would have flipped `isLoading = false`. Not worth a defensive branch.
- `loadError` type narrowed from `string | null` to `string` (empty string == "no error"). No consumer reads this rune outside the template.

**Notes**: String-literal-union form picked (`'loading' | 'ready' | 'error'`) with sibling `loadError` rune for the message payload, per the canonical pattern. State-graph comment block matches the pattern's convention. Both components' transitions are explicit; no implicit flips. Existing tests assert against rendered text and pass unmodified (624/624). `svelte-check` clean (0 errors, 2 pre-existing warnings unrelated). `npm run build` clean.
