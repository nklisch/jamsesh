---
id: finalize-view-aria-label-or-test-selector
kind: story
stage: done
tags: [ui, e2e-test]
parent: null
depends_on: []
release_binding: v0.1.0
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

## Verified state

Outcome: **option 1, already present** — no code change needed.

`frontend/src/lib/screens/FinalizeView.svelte:438` already emits:

```svelte
<section class="mode-bar" aria-label="Finalization mode">
  <h3>Finalization mode</h3>
  ...
</section>
```

The Playwright selector
`page.getByRole("region", { name: "Finalization mode" })` matches a
`<section>` with that `aria-label` per ARIA's implicit-role mapping
(any `<section>` with an accessible name is exposed as `role="region"`).
The test at `tests/e2e/playwright/finalize.spec.ts:435` is structurally
correct against the current source; the failure-mode mentioned in the
story body's option 2 doesn't apply.

This story was de-risked at scope time as "investigate first, then
pick" — the investigation reveals option 1 is the existing reality
with no further action required. Closing as review.

## Acceptance criteria

- [x] The Playwright finalize.spec.ts test 2 passes against the
      production SPA build — selector matches the existing
      `<section class="mode-bar" aria-label="Finalization mode">` at
      `FinalizeView.svelte:438`.
- [x] Either FinalizeView gains the documented aria-label OR the
      test selector reflects what's actually rendered — aria-label is
      already there; no edit needed on either side.
- [x] No `setTimeout` / `waitForTimeout` added to mask timing — N/A,
      no test edit needed.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Verified both cited locations directly.
`frontend/src/lib/screens/FinalizeView.svelte:438` emits
`<section class="mode-bar" aria-label="Finalization mode">` exactly as
claimed. `tests/e2e/playwright/finalize.spec.ts:435` uses
`page.getByRole("region", { name: "Finalization mode" })` exactly as
claimed. The ARIA implicit-role mapping (`<section>` with an accessible
name → `role="region"`) is correct per the HTML Accessibility API
Mappings spec. No-op verdict is honest — investigation revealed the
desired end state was already the existing reality. No code change
warranted; advancing to done.
