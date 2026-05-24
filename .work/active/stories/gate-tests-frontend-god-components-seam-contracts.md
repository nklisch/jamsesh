---
id: gate-tests-frontend-god-components-seam-contracts
kind: story
stage: implementing
tags: [testing, ui, refactor]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Decomposed sub-component seams (props in / events out) not asserted in isolation

## Priority
Medium

## Spec reference
Item: `feature-refactor-frontend-god-components`

Acceptance criterion: Refactor target is behavior-preserving decomposition. Existing tests verify the parent screen still works; nothing asserts the extracted child fulfills its declared seam.

## Gap type
missing test for e2e-seam

## Suggested test
For each extracted child (`useFinalizeCuration`, `useFinalizeExecution`,
`useFinalizeLock`, `useFinalizePlan`, `useCommentComposer`, `useTreeState`,
`useRefActions`, `usePlaygroundCountdown`, `useNewSessionForm`) — add a smoke
test that calls the hook in isolation and asserts the contract documented in
the story body.

## Test location (suggested)
`frontend/src/lib/finalize/`, `frontend/src/lib/session/`, `frontend/src/lib/components/`
