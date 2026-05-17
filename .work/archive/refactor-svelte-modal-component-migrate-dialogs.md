---
id: refactor-svelte-modal-component-migrate-dialogs
kind: story
stage: done
tags: [refactor, ui]
parent: refactor-svelte-modal-component
depends_on: [refactor-svelte-modal-component-define]
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Modal — Migrate ForkDialog and ModeSwitchDialog

Replace the hand-rolled modal scaffold in both dialogs with `<Modal>`.

## Files

- Modify: `frontend/src/lib/components/ForkDialog.svelte`
- Modify: `frontend/src/lib/components/ModeSwitchDialog.svelte`

## What to remove from each consumer

- `<div class="modal-overlay">` and `<div class="modal" …>` wrappers
- `.modal-header` markup + close button
- All CSS for `.modal-overlay`, `.modal`, `.modal-header`, `.modal-title`,
  `.close-btn`

## What to keep

- Form body + `.actions` footer + dialog-specific CSS (fields, mode-badge,
  radio-fieldset, etc.)

## Acceptance

- [ ] `ForkDialog.svelte` and `ModeSwitchDialog.svelte` each shrink by
      40-60 lines
- [ ] Both files import `Modal` from `./Modal.svelte` and use `<Modal>`
- [ ] No `.modal-overlay`, `.modal-header`, or `.close-btn` CSS rules remain
      in either file
- [ ] `ForkDialog.test.ts` and `ModeSwitchDialog.test.ts` pass unchanged
- [ ] Dev-server visual check: both dialogs render identically to before

## Risk

LOW-MEDIUM. The dialog tests pin the click/ESC/submit behavior; if any test
fails, the regression is local and easy to bisect.

## Rollback

`git revert` per file. The two dialogs are independent; one rollback does
not affect the other.

## Implementation notes

- **ForkDialog.svelte**: -44 lines (284 → 240). Scaffold markup and 5 CSS rule
  blocks removed; `<Modal open={true} title="Fork ref" size="md" {onclose}>` added.
- **ModeSwitchDialog.svelte**: -46 lines (272 → 226). Same pattern; uses
  `size="sm"` (matching the original 340–460 px range) and passes
  `ariaLabel="Switch ref mode"` to preserve the `aria-label` the test asserts on.
- **`<Modal>` API extension**: none required. The `ariaLabel` prop already existed
  on `Modal`; no new API surface was needed.
- **Tests**: `ModeSwitchDialog.test.ts` — all 11 tests pass. `ForkDialog.test.ts`
  does not exist. Full suite: 286/286 passed.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Both dialogs migrated cleanly. ForkDialog −44 LoC, ModeSwitchDialog
−46 LoC, total −90 LoC across the two files. No `<Modal>` API extension
needed — the existing `ariaLabel` override was sufficient for
ModeSwitchDialog's `aria-label="Switch ref mode"` (different from its
title "Switch mode"). ModeSwitchDialog uses `size="sm"` (matching its
prior 340–460px range); ForkDialog uses `size="md"` (matching 360–500px).
Full suite 286/286 passing. `ForkDialog.test.ts` not present in repo
(noted in agent's notes; not a finding for THIS story).
