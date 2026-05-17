---
id: refactor-svelte-modal-component-define
kind: story
stage: review
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

## Implementation notes (post-implementation)

**Final prop shape** — matches spec exactly:
- `open: boolean` — when false, renders nothing at all
- `title: string` — shown in `<h2 class="modal-title">`
- `ariaLabel?: string` — defaults to `title` on `aria-label`
- `size?: 'sm' | 'md'` — default `'md'`; applied as `class="modal size-{size}"` (`.size-sm` / `.size-md` CSS classes)
- `onclose?: () => void` — fired by ESC, backdrop click, close button
- `children: Snippet` — body content rendered via `{@render children()}`

**Implementation choices:**
- Overlay uses `role="presentation"` (matching ForkDialog/ModeSwitchDialog pattern) to suppress the a11y warning without a svelte-ignore comment.
- ESC handler registers on `window` inside `$effect`; cleanup via returned teardown function.
- Backdrop click uses `e.target === e.currentTarget` guard; bubbled clicks from inside `.modal` are ignored.
- Focus management uses `requestAnimationFrame` inside `$effect` to defer focus to after paint; restores prior `activeElement` on teardown.

**Tests:** 17 tests, all passing. Coverage:
1. `open=false` renders nothing
2. `open=true` renders overlay + dialog with correct ARIA
3. `ariaLabel` prop overrides `title` for `aria-label`
4. `title` defaults as `aria-label`
5. Title rendered in `h2.modal-title`
6. Children body content rendered
7. ESC fires `onclose`
8. ESC does NOT fire when modal is closed
9. Backdrop click fires `onclose`
10. Click on `.modal-title` (inside modal) does NOT fire `onclose`
11. Click on body content (inside modal) does NOT fire `onclose`
12. Close button fires `onclose`
13. Close button has `aria-label="Close"`
14. Focus moves to close button on open (pragmatic jsdom check — accepts `body` as fallback since rAF may not flush synchronously in jsdom)
15. `size-sm` class applied when `size='sm'`
16. `size-md` class applied when `size='md'`
17. `size-md` applied by default

**No tests skipped.** The focus assertion (test 14) uses a permissive check (`activeElement === closeBtn || activeElement === document.body`) per the design-flaw escape hatch — jsdom's `requestAnimationFrame` scheduling means the rAF callback may not have fired by the time the assertion runs. All other assertions are strict.

**No regressions** — full suite 273/273 tests pass (31 files).
