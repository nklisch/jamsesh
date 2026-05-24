---
id: story-refactor-frontend-god-components-new-session-drawer
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

# Decompose NewSessionDrawer's form + CLI output generator

## Brief

`frontend/src/lib/components/NewSessionDrawer.svelte` is 566 lines. It
combines a session-create form with CLI-command output generation —
two distinct concerns.

## Extraction targets

Read the file first. Likely splits:

1. **`useNewSessionForm.svelte.ts`** — rune module for form state
   (input values, validation, dirty/error tracking). Use
   `wrapper-object-rune-store`.

2. **`SessionCommandPreview.svelte`** — CLI command output component
   that consumes the form's state and renders the formatted
   `jamsesh new ...` command. Pure rendering — takes form values as
   props and outputs the command string + copy-to-clipboard handler.

3. **`parseCommaSeparated` helper** — the comma-list parsing logic
   (referenced in the per-feature design discovery as a candidate for
   a shared `string-utils.ts`). If it's clearly the same shape as the
   one in `ForkDialog.svelte`, extract to `$lib/string-utils.ts`;
   otherwise leave it inline for now.

## Acceptance criteria

- [ ] `NewSessionDrawer.svelte` LoC ≤ 300.
- [ ] Form state extracted to a rune module OR the CLI-output renderer
      extracted to a sub-component (whichever is the cleaner cut for
      this file's actual structure).
- [ ] No visible UI change — same form fields, same CLI output, same
      submit handlers.
- [ ] `npm run check` clean.
- [ ] `npm run test` passes.
- [ ] `npm run build` clean.

## Risk

**Low.** Form + output are naturally separable.

## Rollback

`git revert` the commit.
