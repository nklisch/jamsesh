---
id: idea-app-test-svelte5-component-mock-broken
kind: story
stage: done
tags: [testing, ui]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-25
---

## Land mode

Verified at autopilot-drain time: `App.test.ts` already passes (15 tests
passed, 1 skipped). The screen-component mock factories now use Svelte 5-
compatible callable stubs:

```ts
vi.mock('$lib/screens/Login.svelte', () => ({
  default: function LoginStub(_anchor: unknown, _props: unknown) { return {}; },
}));
```

and the spy-tracked stubs use `(...args) => mockX(...args)` indirection
via the `spa-test-module-mock-barrel` pattern. The story's described
breakage was resolved in a prior pass (likely during the
`story-portal-visitor-entry-pages-spa-landing` work that landed
`App.svelte` mocks at scale).

No code change needed for this story.

Verified: `npm test -- --run App.test.ts` → 15 passed, 1 skipped.

App.test.ts has 9 pre-existing failures with "TypeError: default is not a function" whenever App.svelte renders a child screen component (Login, Home, MagicLinkExchange, OAuthCallback, SessionList, InviteAccept). The error surfaces in the Svelte 5 compiled `add_svelte_meta.componentTag` binding, suggesting the `vi.mock(...)` factories for those screen components return plain objects rather than callable constructors — Svelte 5's compiled template expects the default export to be a function/class. These failures pre-date the current session and are not caused by any Home.test.ts or OAuthCallback.test.ts edits; they need the App.test.ts mock factories audited and fixed to return Svelte 5-compatible component stubs.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Land-mode story; the described breakage was already resolved upstream. Verification (15 passed, 1 skipped) confirms current state matches the story's land claim. No diff to review — story is documentation of state.
