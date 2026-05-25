---
id: bug-app-home-renders-during-portalinfo-loading-flash
kind: story
stage: implementing
tags: [bug, ui, regression, flash, a11y]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# App.svelte flashes `<Home/>` (org picker) before portalInfo resolves on unauthed `/`

## Brief

When an anonymous visitor lands on `/` and `portalInfo.init()` has not yet
resolved, App.svelte's template falls through the `{:else if current.name ===
'home' && !auth.isAuthenticated && portalInfo.loaded && portalInfo.landingVariant
=== 'project'}` branch (the `portalInfo.loaded` clause short-circuits) and
lands on the next branch `{:else if current.name === 'home'}` which renders
`<Home/>` — the org picker. The user sees a brief flash of the wrong UI before
the bootstrap fetch resolves and the correct landing screen takes over.

The spec for `feature-portal-visitor-entry-pages` explicitly promised this
would not happen: "render a tiny loading shell (transparent) until both
`auth.init()` and `portalInfo.init()` resolve." The `$effect`-based auth-gate
in App.svelte already holds correctly (returns early when `!portalInfo.loaded`
on the home route), but the template gate is missing the same guard.

## Discovered by

`gate-tests-app-gate-flash-unauthed-portalinfo-not-loaded` — the sentinel
test landed at `frontend/src/App.test.ts:319-336` as `it.skip` with the
comment "Skipped: known production bug
`bug-app-home-renders-during-portalinfo-loading-flash`". The test rewires
the `Home.svelte` and `ProjectLanding.svelte` mocks through module-level
`vi.fn()` spies (`mockHomeStub`, `mockProjectLandingStub`) so the template's
mount decision becomes observable, then asserts neither stub is invoked
during the flash window. The test will pass automatically once the template
gate is fixed.

## Root cause

`frontend/src/App.svelte:90-93`:

```svelte
{:else if current.name === 'home' && !auth.isAuthenticated && portalInfo.loaded && portalInfo.landingVariant === 'project'}
  <ProjectLanding />
{:else if current.name === 'home'}
  <Home />
```

The second `{:else if}` matches when:
- `current.name === 'home'` ✓
- `!auth.isAuthenticated` ✓ (anonymous visitor)
- `portalInfo.loaded === false` (still bootstrapping)

In this window the auth-gate `$effect` returns early (line 51:
`if (!portalInfo.loaded) return;`), so no navigation happens, but the
template still has to render *something* — and it picks `<Home/>`. The
auth-gate prevents the URL change but not the brief org-picker flash.

The fix-direction the spec implied is to render nothing (or a transparent
loading shell) during this window. Returning nothing from the template
is the simplest expression of "transparent loading shell" — the DOM stays
empty until both bootstraps resolve and the right branch lights up.

## Fix shape

Tighten the generic home branch so it only fires when one of the
preconditions for a real landing decision has been met:

```svelte
{:else if current.name === 'home' && (auth.isAuthenticated || portalInfo.loaded)}
  <Home />
```

Branch behavior after the fix:

| auth.isAuthenticated | portalInfo.loaded | landingVariant     | Result                               |
|---                   |---                |---                 |---                                   |
| true                 | (any)             | (any)              | Home (line 92)                       |
| false                | true              | project            | ProjectLanding (90)                  |
| false                | true              | login / auto       | auth-gate navigates away; line 92 won't match because the gate-driven `navigate('/login')` flips `current.name` |
| false                | false             | (any)              | **Nothing — empty loading shell, the spec promise** |

The single new clause `(auth.isAuthenticated || portalInfo.loaded)` covers
both legitimate render paths (authed regardless of portalInfo, or unauthed
once portalInfo has resolved). The flash window — unauthed AND not loaded
— intentionally falls through to the implicit empty render.

## Alternative considered (rejected)

**Explicit loading-shell branch**:

```svelte
{:else if current.name === 'home' && !auth.isAuthenticated && !portalInfo.loaded}
  <!-- transparent loading shell -->
{:else if current.name === 'home'}
  <Home />
```

Adds a named branch but requires an empty `{:else if}` body (or a thin
spinner element) which then needs its own a11y / aria-busy treatment. The
spec promise is "transparent" so empty DOM matches the intent. The
tightening approach is one clause, no new branch, no new elements.

## Acceptance criteria

- [ ] `App.svelte:92` becomes
  `{:else if current.name === 'home' && (auth.isAuthenticated || portalInfo.loaded)}`
- [ ] The skipped sentinel
  `it.skip('unauthed + portalInfo.loaded=false → neither Home nor ProjectLanding mounts (flash gate)')`
  in `frontend/src/App.test.ts` is un-skipped (`.skip` removed) and passes
- [ ] The inline skip-comment naming this bug id is removed
- [ ] No existing test in `App.test.ts` regresses (15/16 pass + 1 unskipped
  → 16/16 pass)
- [ ] The auth-gate `$effect` for navigation remains unchanged — the fix is
  template-only
- [ ] `npm run check` (svelte-check) clean for App.svelte

## Test plan

The sentinel test already exists. After the fix:

1. Remove `.skip` and the linked-bug comment from the sentinel.
2. Run `cd frontend && npm test -- --run App.test.ts` — assert all
   pass (16 tests, 0 skipped) including the un-skipped sentinel.
3. (Optional, lightweight) Add a positive twin: `it('unauthed + portalInfo.loaded=true + landingVariant=login → does not mount Home or ProjectLanding before navigate fires')`.
   The existing test `'portalInfo not yet loaded → does not navigate until loaded (gate holds)'` already covers the loaded=false case from the
   $effect side; combined with the sentinel they fully pin the gate.

## Files touched (anticipated)

- `frontend/src/App.svelte` — one-clause edit at line 92
- `frontend/src/App.test.ts` — remove `.skip` + the `// Skipped: ...` comment on lines ~321-326
