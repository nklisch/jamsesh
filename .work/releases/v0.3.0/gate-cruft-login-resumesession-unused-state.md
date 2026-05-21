---
id: gate-cruft-login-resumesession-unused-state
kind: story
stage: done
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

## Implementation notes

**Grep findings:** `resumeSession` appears at exactly 3 sites in `Login.svelte`:
- Line 26: declaration (changed)
- Line 113: `{#if resumeSession}` — read-only template branch
- Line 118: `{resumeSession}` interpolation — read-only

Zero write sites confirmed. The `$state` wrap was unnecessary overhead.

**Change applied:** `frontend/src/lib/screens/Login.svelte` line 26

Before:
```ts
let resumeSession = $state<string | null>(_searchParams.get('resume'));
```

After:
```ts
const resumeSession: string | null = _searchParams.get('resume');
```

This follows the existing precedent of `_returnTo` / `returnTo` (lines 37–41)
which are also plain `const`s initialized from URL params.

**Verification:**
- `npm run check`: 0 errors, 2 pre-existing warnings (unrelated)
- `npm test`: 465/465 tests pass, including Login.test.ts resume-strip coverage

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: `$state` → `const` for a read-only-at-mount value. Matches the existing `_returnTo` / `returnTo` precedent in the same file (lines 37–41). Grep confirmed zero write sites for `resumeSession`. Template reads at lines 113, 118 work unchanged. 465→465 tests pass.
