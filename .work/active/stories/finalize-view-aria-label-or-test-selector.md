---
id: finalize-view-aria-label-or-test-selector
kind: story
stage: drafting
tags: [ui, e2e-test]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Finalize view: verify aria-label or update Playwright selector

## Finding

`tests/e2e/playwright/finalize.spec.ts` test 2 uses
`page.getByRole("region", { name: "Finalization mode" })` (or similar)
which requires the FinalizeView's mode-bar `<section>` to carry
`aria-label="Finalization mode"`.

The implementing agent noted this dependency but didn't verify the
Svelte source actually emits the attribute. Either:
1. The aria-label is already there → the test passes → no action
2. The aria-label is missing → the test fails on first run

## Suggested resolution

Pick one based on what `frontend/src/lib/screens/FinalizeView*.svelte`
actually renders:

1. **Add the aria-label** to the mode-bar section in
   `FinalizeView.svelte` — improves accessibility AND fixes the test
2. **Update the selector** in `tests/e2e/playwright/finalize.spec.ts`
   to use whatever stable selector the section actually exposes
   (e.g., `.mode-bar` class, or a different role+name)

Option 1 is the better UX outcome.

## Acceptance criteria

- [ ] The Playwright finalize.spec.ts test 2 passes against the
      production SPA build
- [ ] Either FinalizeView gains the documented aria-label OR the
      test selector reflects what's actually rendered
- [ ] No `setTimeout` / `waitForTimeout` added to mask timing
