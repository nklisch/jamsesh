---
id: gate-cruft-app-stale-later-story-comment
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
