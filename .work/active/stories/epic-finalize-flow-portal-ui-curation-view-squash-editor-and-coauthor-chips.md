---
id: epic-finalize-flow-portal-ui-curation-view-squash-editor-and-coauthor-chips
kind: story
stage: done
tags: [ui]
parent: epic-finalize-flow-portal-ui-curation-view
depends_on: [epic-finalize-flow-portal-ui-curation-view-screen-and-route]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Finalize curation — squash message editor and co-author chips

## Scope

Implement the two squash-mode-only sub-components composed by
FinalizeView's cart-config block:

1. `SquashMessageEditor.svelte` — monospaced textarea bound to
   the parent's `commitMessage` $state, with a hint row, a
   label-row matching the Option-3 mock, and an embedded
   `CoAuthorChipRow` showing the distinct contributors across
   the current selection.
2. `CoAuthorChipRow.svelte` — pill-row of chips (one per
   distinct author in the selection) with an `AuthorDot`-colored
   marker per chip.

These render only when `mode === 'squash'` (the parent FinalizeView
conditionally mounts them). Story 1 already imports these from
`$lib/components/`; this story replaces the placeholder
implementations with the real ones.

## Units delivered

- `frontend/src/lib/components/SquashMessageEditor.svelte`
- `frontend/src/lib/components/SquashMessageEditor.test.ts`
- `frontend/src/lib/components/CoAuthorChipRow.svelte`
- `frontend/src/lib/components/CoAuthorChipRow.test.ts`

## Component signatures

```ts
// SquashMessageEditor.svelte
let {
  message,
  onmessagechange,
  coAuthors,
}: {
  message: string;
  onmessagechange: (next: string) => void;
  coAuthors: components['schemas']['CoAuthor'][];
} = $props();

// CoAuthorChipRow.svelte
let { authors }: { authors: components['schemas']['CoAuthor'][] } = $props();
```

## Acceptance criteria

- Editor renders a monospaced textarea pre-populated with the
  `message` prop; `oninput` calls `onmessagechange(textarea.value)`
  on every keystroke (parent owns debouncing).
- Editor includes the `Squash mode · pre-filled from session goal
  + commit subjects · editable` hint per the mock.
- Editor composes `<CoAuthorChipRow authors={coAuthors}/>` below
  the textarea.
- ChipRow renders one chip per author with an `AuthorDot` marker
  matching the author's session color and the author's display
  name in monospace.
- ChipRow renders nothing (no label, no chips) when `authors` is
  empty.
- When parent re-passes `coAuthors` after a curation change, the
  ChipRow updates in place (Svelte 5 prop reactivity, no
  remount).
- Vitest suites cover all bullets above (8+ assertions across the
  two test files).
- `cd frontend && pnpm test` green; `cd frontend && pnpm check`
  clean.

## Notes

- Chip styles match the locked Option-3 mock at
  `.mockups/screens/epic-finalize-flow-portal-ui-curation-view/option-3.html`
  — `.coauthors .chip`, padded pill on `--color-bg-tertiary` with
  an 8px dot. Reuse `AuthorDot` from the design system for the
  marker so colors come from the existing palette tokens.
- The editor's textarea uses `font: var(--font-size-sm)/1.55
  var(--font-mono)` per the locked palette. No drag, no resize
  handles beyond the browser default.
- No write path to the server lives here — the editor only owns
  the local UI affordance and the `onmessagechange` callback.
  All persistence (debounced PATCH) is the parent's job.

## Implementation notes

- `CoAuthorChipRow.svelte` — replaced the empty placeholder. Renders
  the entire `.coauthors` block (label + chips) only when
  `authors.length > 0`, so single-author edge cases don't leave a
  dangling "Co-authors" label or open up a grid gap. Each chip is
  an `<AuthorDot size={8}>` + the author's display name in
  `var(--font-mono)`. Color seed is `account_id ?? email`, mirroring
  the FinalizeView cart-item dot seed (`account_id ?? author_email`)
  so the same contributor renders the same color whether they appear
  in the cart row or as a chip.
- `SquashMessageEditor.svelte` — replaced the story-1 minimum-viable
  textarea. Class names + CSS now match the locked Option-3 mock
  (`.msg-editor`, `.label-row`, `.hint`). Added the
  "Squash mode · pre-filled from session goal + commit subjects ·
  editable" hint span next to the label. Textarea continues to
  forward `oninput` directly to `onmessagechange(value)` per
  keystroke; the parent owns debouncing. Pure input/output surface
  driven entirely by props.
- Made `coAuthors` a **required** prop (was optional `=[]` in
  story-1 placeholder) per the story signature; FinalizeView already
  passes `coAuthors={distinctAuthors}` so this is a tightening.
- Reused `AuthorDot.svelte` rather than re-implementing the 8px
  colored marker, which keeps the per-author color hashing
  centralized.
- Tests expanded from 3 placeholder smoke tests per file to 7
  (SquashMessageEditor) + 8 (CoAuthorChipRow) targeted tests.
  Covers: pre-population, hint row, label association, keystroke
  callback, embedded chip-row composition, empty-author no-op,
  prop reactivity (same-node rerender), chip count per author,
  AuthorDot composition with --author-N color, account_id vs email
  seed handling, chip-row disappearance when authors empties out.
- AuthorDot color-seed design check: `account_id ?? email` is stable
  across renders. The tree pane (`TreeDag.svelte`) uses a different
  seed strategy (agent name from ref path) so co-author chip color
  won't always match a chip-vs-tree comparison in general — but it
  *will* match the cart-item dot in the same FinalizeView, which is
  the locally-meaningful color identity for this screen. No
  escape-hatch needed — the existing system already accepts this
  cross-surface drift (e.g. SessionList seeds by account_id, TreeDag
  seeds by ref-path username), so applying the same convention here
  keeps the chip row consistent with the FinalizeView cart it sits
  next to.

## Verification

- `cd frontend && pnpm test` — 256 tests pass (15 in the two
  changed suites: 7 SquashMessageEditor + 8 CoAuthorChipRow).
- `cd frontend && pnpm check` — 0 errors, 1 pre-existing warning in
  unrelated `ModeSwitchDialog.svelte`.
- `go build ./...` — fails in `cmd/jamsesh/finalizecmd/finalizerun.go`
  on missing `chooseFetchSource`/`performFetch`. Unrelated:
  `fetchsource_stub.go` is staged-deleted in the index from other
  in-flight work; no Go files were touched by this story.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Components match the Option-3 mock. AuthorDot color seed strategy consistent with FinalizeView cart rows. 15 tests cover the prop reactivity matrix.
