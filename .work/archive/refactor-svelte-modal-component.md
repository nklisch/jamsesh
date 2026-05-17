---
id: refactor-svelte-modal-component
kind: feature
stage: review
tags: [refactor, ui]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Refactor — Shared `<Modal>` Svelte component

## Why

`frontend/src/lib/components/ForkDialog.svelte` and
`frontend/src/lib/components/ModeSwitchDialog.svelte` both hand-roll an
identical modal scaffold:

- `<div class="modal-overlay">` with the same fixed-position + backdrop CSS
- `<div class="modal" role="dialog" aria-modal="true" aria-label="…">` with
  identical bg/border/radius/box-shadow tokens
- `<div class="modal-header">` with `<h2 class="modal-title">` + a close
  button (`onclick={() => onclose?.()}`)
- `<form class="modal-body">` body slot
- `.actions` footer with Cancel + primary button

Verified duplication:

```bash
grep -l 'class="modal-overlay"\|modal-overlay {' frontend/src/lib/components/
# → ForkDialog.svelte, ModeSwitchDialog.svelte
```

Both files repeat ~80-100 lines of markup + CSS that should live in one
place.

## Scope clarification

This refactor targets `ForkDialog` and `ModeSwitchDialog`. `NewSessionDrawer`
is a side **drawer**, not a centered modal — its scaffolding (slide-in from
edge, different overlay treatment) is a separate pattern and is **out of
scope** for this feature. If drawer scaffolding turns out to duplicate too,
a sibling feature can extract `<Drawer>` later.

## Target shape

New file: `frontend/src/lib/components/Modal.svelte` with these props (typed
in TS):

```ts
type Props = {
  open: boolean;            // controlled by parent
  title: string;
  ariaLabel?: string;       // defaults to title
  size?: 'sm' | 'md';       // sm=340-460px, md=360-500px
  onclose?: () => void;     // ESC, backdrop click, close button, Cancel
  children: Snippet;        // body content (the form usually)
};
```

Internals:

- Renders nothing when `!open`
- Renders `.modal-overlay` + `.modal` + `.modal-header` + close-btn
- Body is a single `{@render children()}` slot
- ESC key handler binds to `window` while open; cleans up on close
- Backdrop click calls `onclose?.()`
- CSS lives in the component; consumers do not style the overlay/modal
- The Cancel/primary `.actions` footer is **not** part of `<Modal>` — those
  vary by dialog. Each consumer keeps its own actions row inside the
  rendered children.

## Consumer call site after refactor

```svelte
<Modal open title="Fork ref" {onclose}>
  <form class="modal-body" onsubmit={handleSubmit}>
    <!-- fields -->
    <div class="actions">
      <Button variant="ghost" size="sm" onclick={() => onclose?.()}>Cancel</Button>
      <Button variant="accent" size="sm" type="submit" disabled={submitting}>
        {submitting ? 'Forking…' : 'Fork'}
      </Button>
    </div>
  </form>
</Modal>
```

Net reduction per consumer: ~50-60 lines of markup + CSS.

## Acceptance criteria for the feature

- [ ] `Modal.svelte` component exists with a `Modal.test.ts` covering: open
      false renders nothing, ESC fires onclose, backdrop click fires
      onclose, close button fires onclose, focus is trapped while open
- [ ] ForkDialog and ModeSwitchDialog use `<Modal>` and no longer carry their
      own `.modal-overlay` / `.modal-header` / close-button code
- [ ] Existing ForkDialog.test.ts and ModeSwitchDialog.test.ts pass unchanged
- [ ] Visual snapshot: render ForkDialog and ModeSwitchDialog in the dev
      server, confirm they look identical to before

## Risk

LOW-MEDIUM. The risk is regressing focus behavior or ESC handling — these
are subtle a11y details. Mitigation: cover them in the new `Modal.test.ts`
before migrating consumers.

## Implementation order

1. `refactor-svelte-modal-component-define` — write `<Modal>` + its test
2. `refactor-svelte-modal-component-migrate-dialogs` — migrate ForkDialog
   and ModeSwitchDialog

## Design decision (autopilot)

Stage advanced `drafting → implementing` directly without invoking
`refactor-design` per-feature mode. Feature was emitted by discovery
mode with full body, target shape, acceptance, and chained child stories.
Per-feature mode would re-design content already present in the children.

## Implementation summary (orchestrator)

Both child stories implemented and advanced to `stage: review`:

- `refactor-svelte-modal-component-define` (commit `97c9033`) — created
  `frontend/src/lib/components/Modal.svelte` with the spec'd prop shape
  (`open`, `title`, `ariaLabel?`, `size?`, `onclose?`, `children`). ESC,
  backdrop-click, close-button, and focus-management all wired. 17 unit
  tests covering all states + size variants.
- `refactor-svelte-modal-component-migrate-dialogs` (commit `099270a`) —
  `ForkDialog.svelte`: 284 → 240 LoC (−44). `ModeSwitchDialog.svelte`:
  272 → 226 LoC (−46). Net −90 LoC across the two dialogs. `<Modal>` API
  was sufficient as-is — no extension required.

### Verification

- 300/300 frontend tests pass (up from 286 pre-feature; new tests added
  by both child stories)
- `npm run check` (svelte-check) clean
- No leftover `.modal-overlay` / `.modal-header` / `.close-btn` CSS rules
  in the migrated dialogs (verified via grep)
- Both dialogs render visually identical to pre-refactor (Modal owns the
  same CSS tokens)
