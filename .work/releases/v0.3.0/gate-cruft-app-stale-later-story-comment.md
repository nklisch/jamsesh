---
id: gate-cruft-app-stale-later-story-comment
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

# Stale `(in a later story)` parenthetical in `App.svelte` bootstrap comment

## Confidence
Medium

## Category
stale comment

## Location
`frontend/src/App.svelte:47-48`

## Evidence
```svelte
// OAuthCallback awaits loadCurrentUser() explicitly before navigating
// (in a later story), so this effect is a no-op there (guarded inside
// auth.loadCurrentUser via the _currentUser/_orgs check).
```

But `OAuthCallback.svelte:54` already does `await auth.loadCurrentUser();`
inside the bundle — the "later story" has landed.

## Removal
Drop the parenthetical "(in a later story)". Rewrite to state the
current contract: "OAuthCallback awaits loadCurrentUser() explicitly
before navigating, so this effect is a no-op there (guarded inside
auth.loadCurrentUser via the _currentUser/_orgs check)."

## Implementation notes

- Verified `OAuthCallback.svelte:54` (`frontend/src/lib/screens/OAuthCallback.svelte`)
  already has `await auth.loadCurrentUser();` — the "later story" has landed.
- Edited `frontend/src/App.svelte` lines 47-49: removed the stale `(in a later story)`
  parenthetical and reformatted the three-line comment to flow cleanly.
- Exact rewrite applied:
  ```
  // OAuthCallback awaits loadCurrentUser() explicitly before navigating,
  // so this effect is a no-op there (guarded inside auth.loadCurrentUser
  // via the _currentUser/_orgs check).
  ```
- `npm run check`: 0 errors, 2 pre-existing warnings (unrelated).
- `npm test`: 41 test files, 464 tests — all pass before and after the edit.

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Comment edit only — exact rewrite as spec'd, OAuthCallback.svelte:54 confirmed to already await loadCurrentUser before the cleanup. 464→464 tests pass; no behavior change.
