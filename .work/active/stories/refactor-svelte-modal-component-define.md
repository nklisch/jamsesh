---
id: refactor-svelte-modal-component-define
kind: story
stage: implementing
tags: [refactor, ui]
parent: refactor-svelte-modal-component
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Modal — Define component

Write `Modal.svelte` + its test. Do not migrate any consumers in this story.

## Files

- New: `frontend/src/lib/components/Modal.svelte`
- New: `frontend/src/lib/components/Modal.test.ts`

## Implementation notes

- Use Svelte 5 runes (`$props`, `$state`) and snippet children
- ESC binding via `$effect` registering a `window.addEventListener('keydown', …)`
  with a cleanup function
- Backdrop click vs modal click: detect by `event.target === event.currentTarget`
  on the overlay; ignore clicks that bubble from inside `.modal`
- CSS pulls the same tokens already in use (`var(--color-bg-secondary)`,
  `var(--color-border-strong)`, `var(--radius-md)`)
- Focus: on open, move focus to the close button; on close, restore focus
  to the previously-active element

## Acceptance

- [ ] `Modal.svelte` exports default with the prop shape from the parent
      feature body
- [ ] `Modal.test.ts` covers: open=false renders nothing, ESC fires onclose,
      backdrop click fires onclose, click inside modal does NOT fire
      onclose, close button fires onclose
- [ ] `pnpm test` (or `npm test`) under `frontend/` passes
- [ ] No existing component imports `Modal` yet — that's the next story

## Risk

LOW.

## Rollback

`git revert`; the file is new and uncoupled.
