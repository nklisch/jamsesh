---
id: gate-tests-xss-activityfeed-component
kind: story
stage: implementing
tags: [testing, security, ui]
parent: null
depends_on: [gate-security-xss-html-render-ws-events]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Stored XSS in `ActivityFeed.svelte` has no end-to-end or component test

## Priority
Critical

## Spec reference
Item: `gate-security-xss-html-render-ws-events`
Acceptance criterion: payload `<img src=x onerror=...>` posted as a
comment body must NOT execute when broadcast in `comment.added` events.

## Gap type
missing test for adversarial-spec-silent. `ActivityFeed.test.ts` exists
but contains no XSS / script-tag / sanitize assertion.

## Suggested test
```ts
// ActivityFeed.test.ts
it('renders comment body containing <script> tag as text, not HTML', async () => {
  // Feed an event {type:'comment.added', payload:{author_id:'a', body:'<img src=x onerror="window.__pwned=1">'}}.
  // After render, assert document.body.innerHTML does NOT contain an actual <img onerror=>.
  // Assert window.__pwned is undefined.
});
```

## Test location (suggested)
`frontend/src/lib/components/ActivityFeed.test.ts`
