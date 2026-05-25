---
id: idea-app-test-svelte5-component-mock-broken
created: 2026-05-20
tags: [testing]
---

App.test.ts has 9 pre-existing failures with "TypeError: default is not a function" whenever App.svelte renders a child screen component (Login, Home, MagicLinkExchange, OAuthCallback, SessionList, InviteAccept). The error surfaces in the Svelte 5 compiled `add_svelte_meta.componentTag` binding, suggesting the `vi.mock(...)` factories for those screen components return plain objects rather than callable constructors — Svelte 5's compiled template expects the default export to be a function/class. These failures pre-date the current session and are not caused by any Home.test.ts or OAuthCallback.test.ts edits; they need the App.test.ts mock factories audited and fixed to return Svelte 5-compatible component stubs.
