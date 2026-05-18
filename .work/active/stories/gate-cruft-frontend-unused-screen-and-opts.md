---
id: gate-cruft-frontend-unused-screen-and-opts
kind: story
stage: implementing
tags: [cleanup, ui]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# Unused `screen` import and `opts` parameter in test files

## Confidence
High

## Category
unused import / unused parameter (TS6133)

## Location
- `frontend/src/lib/components/TreeDag.test.ts:2` — `screen` from `@testing-library/svelte`
- `frontend/src/lib/screens/FinalizeView.test.ts:213` — `opts` mock parameter

## Evidence
```
TreeDag.test.ts:2 — 'screen' is declared but its value is never read.
FinalizeView.test.ts:213 — 'opts' is declared but its value is never read.
```

## Removal
- `TreeDag.test.ts`: drop `screen` from the import (keep `render`).
- `FinalizeView.test.ts`: rename `opts` to `_opts` or drop entirely if
  the mock signature doesn't need to mirror the production type.
