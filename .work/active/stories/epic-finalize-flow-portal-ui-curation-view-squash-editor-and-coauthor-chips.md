---
id: epic-finalize-flow-portal-ui-curation-view-squash-editor-and-coauthor-chips
kind: story
stage: implementing
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
