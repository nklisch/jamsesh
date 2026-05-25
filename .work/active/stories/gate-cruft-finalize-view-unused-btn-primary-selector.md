---
id: gate-cruft-finalize-view-unused-btn-primary-selector
kind: story
stage: review
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# FinalizeView.svelte: unused .btn.primary CSS selector

## Confidence
High

## Category
unused import

## Location
`frontend/src/lib/screens/FinalizeView.svelte:689-693`

## Evidence
```css
.btn.primary {
  background: var(--color-bg-inverse); color: var(--color-text-inverse);
  border: 1px solid var(--color-bg-inverse);
  width: auto; padding: 7px 14px;
}
```

`svelte-check --threshold warning` reports:
```
WARNING "src/lib/screens/FinalizeView.svelte" 689:3 "Unused CSS selector \".btn.primary\""
```

`grep -nE 'class.*primary|class.*btn' frontend/src/lib/screens/FinalizeView.svelte` finds only `class="btn ghost"` and `class="ship-btn"`. No element in this component uses `class="btn primary"`. The selector is residue from an earlier UI variant.

## Removal
Delete the `.btn.primary` block (lines 689-693). The other `.btn` and `.btn.ghost` rules remain — they are still in use. Re-run `pnpm --filter frontend svelte-check` (or the project's lint script) to confirm the warning is gone.

## Implementation notes
Deleted the `.btn.primary` CSS block from `frontend/src/lib/screens/FinalizeView.svelte`. `npm run check` shows 0 errors 1 warning (pre-existing unrelated warning in ModeSwitchDialog). All 693 frontend tests pass.
