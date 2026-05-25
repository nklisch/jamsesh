---
id: gate-tests-portalinfo-init-post-load-resolves-synchronously
kind: story
stage: drafting
tags: [testing, ui, cleanup]
parent: null
depends_on: []
release_binding: v0.4.1
gate_origin: tests
created: 2026-05-25
updated: 2026-05-25
---

# portalInfo store — post-load `init()` synchronous-resolve not asserted

## Priority
Low

## Spec reference
Item: `story-portal-visitor-entry-pages-spa-landing`
Store implementation notes: `init()` is idempotent — *"if _loaded
return Promise.resolve()"*.

## Gap type
Trivial defensive assertion.

## Location
`frontend/src/lib/portalInfo.test.ts:97-110` — "second init() call is
a no-op" asserts the GET wasn't re-called, but doesn't assert the
returned promise resolves to `undefined` without scheduling a fetch.

## Remediation direction
Trivial assertion add to the "second init()" test — `await expect(p2).resolves.toBeUndefined()`
paired with mock-call-count check.
