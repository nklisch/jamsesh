---
id: gate-cruft-login-resumesession-unused-state
kind: story
stage: drafting
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: cruft
created: 2026-05-20
updated: 2026-05-20
---

# `resumeSession` declared as `$state` but never reassigned

## Confidence
Medium

## Category
over-abstraction (unnecessary reactivity wrapper)

## Location
`frontend/src/lib/screens/Login.svelte:26`

## Evidence
```ts
let resumeSession = $state<string | null>(_searchParams.get('resume'));
```

Initialized from URL params at mount and only ever read in the template
(lines 113, 118). No reassignment site — `$state` wrap is unnecessary
overhead.

## Removal
Change to `const resumeSession: string | null = _searchParams.get('resume');`.
