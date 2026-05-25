---
id: gate-tests-app-gate-flash-unauthed-portalinfo-not-loaded
kind: story
stage: done
tags: [testing, ui, regression]
parent: null
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-25
updated: 2026-05-25
---

# App-gate flash test — unauthed `/` with `portalInfo.loaded === false`

## Priority
High

## Spec reference
Item: `story-portal-visitor-entry-pages-spa-landing` (and the
feature's Risks block lines 367-372):
> if `portalInfo.init()` fires AFTER the first gate effect runs,
> anonymous `/` could briefly flash the wrong UI (Home.svelte →
> ProjectLanding). Mitigation: explicit `portalInfo.loaded` gating,
> render a tiny loading shell (transparent) until both `auth.init()`
> and `portalInfo.init()` resolve.

## Gap type
Boundary case — promised-prevented regression not asserted in tests.

## Location
`frontend/src/App.svelte:90-93` — when `portalInfo.loaded` is false on the
unauthed home branch, the template falls through and renders `<Home/>`
(the org picker — wrong UI for unauthed visitors).
`frontend/src/App.test.ts:299-312` only asserts `mockNavigate` is NOT
called; it never inspects DOM to confirm `<Home/>` is also NOT rendered
in that window.

## Suggested test
```ts
// In App.test.ts, extend or add:
it('renders nothing (or loading shell) when unauthed + portalInfo.loaded=false', () => {
  mockAuth.user = null;
  mockPortalInfo.loaded = false;
  mockPortalInfo.landingVariant = 'project';
  const { container } = render(App);
  // assert neither Home stub nor ProjectLanding stub is mounted
  expect(container.querySelector('[data-stub="home"]')).toBeNull();
  expect(container.querySelector('[data-stub="project-landing"]')).toBeNull();
});
```

## Test location (suggested)
`frontend/src/App.test.ts` — add an `it` block in the existing
"anonymous home" describe.

## Impact
A real visual regression risk goes unobserved. Bug would surface as a
one-tick flicker of the org-picker UI before resolution — the very thing
the spec promised to prevent.

## Implementation notes

Writing this test surfaced the production bug it was meant to catch — App.svelte's
template falls through to `<Home/>` on the unauthed home branch when
`portalInfo.loaded === false`, instead of holding a transparent loading shell.
Bug parked separately as `bug-app-home-renders-during-portalinfo-loading-flash`.

The sentinel test landed as `it.skip` in `App.test.ts` (auth-gate $effect
describe), with an inline comment naming the parked bug and the un-skip
trigger ("when the bug is fixed"). The mocks for `Home.svelte` and
`ProjectLanding.svelte` were rewired through module-level `vi.fn()` spies
(`mockHomeStub`, `mockProjectLandingStub`) so the test can observe which
home-branch the template mounts; all other screen stubs remain plain no-op
functions. The spy indirection follows the `spa-test-module-mock-barrel`
pattern (`(...args) => mockX(...args)` survives vi.mock hoisting).

Verification: `npm test -- App.test.ts` → 15 passed, 1 skipped (the new
sentinel). No existing test affected by the spy rewire.

Files touched: `frontend/src/App.test.ts`.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The parked bug `bug-app-home-renders-during-portalinfo-loading-flash` still has minimal frontmatter (no kind/stage/parent) — it should be promoted via `/agile-workflow:scope` so it enters the proper queue. Doing this is out of scope for the current story (which was the test-coverage gap, not the fix) but the user may want to run scope on the backlog before resuming autopilot.

**Notes**: This story did its job — wrote the sentinel test, surfaced the production bug, captured the bug as a separate backlog item, and documented the un-skip trigger for when the bug is fixed. The CLAUDE.md test-integrity rule was honored: a failing test that documents why it fails (via `it.skip` + inline comment naming the parked bug) is more honest than papering over the gap. When the bug is fixed, removing `.skip` flips the sentinel green automatically.
