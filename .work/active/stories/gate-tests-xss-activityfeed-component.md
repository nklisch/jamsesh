---
id: gate-tests-xss-activityfeed-component
kind: story
stage: review
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

## Implementation notes

Two tests were added to `frontend/src/lib/components/ActivityFeed.test.ts`, replacing the `it.todo` placeholder:

**Test 1 — `<img onerror>` payload via `comment.added`**
Fires a `comment.added` event with `body: '<img src=x onerror="window.__pwned=1">'`. After mount via `@testing-library/svelte render()`, asserts:
- `container.querySelector('img')` is `null` — no live `<img>` element was parsed into the DOM. This is the primary DOM-level guard; it fails immediately if `{@html}` is ever reintroduced.
- `container.innerHTML` matches `/&lt;img/i` — the payload was rendered as an escaped text node (not silently dropped). This verifies the fragment pipeline actually rendered the body field, keeping the assertion meaningful.
- `window.__pwned` is `undefined` — the script callback did not fire.

**Test 2 — `<script>` tag payload via `comment.added`**
Fires a `comment.added` event with `body: '<script>window.__pwned=2;</script>'`. After mount, asserts:
- `container.querySelector('script')` is `null` — no script element was injected.
- `container.innerHTML` does not match `/<script/i` — no unescaped opening tag is present.
- `container.innerHTML` matches `/&lt;script/i` — payload was rendered as escaped text, not dropped.
- `window.__pwned` is `undefined`.

**Assertion strategy notes**
- Testing the real `ActivityFeed.svelte` end-to-end — `formatEvent` is not mocked.
- The tests use synchronous DOM assertions after `waitFor` confirms the feed item rendered. No timing dependency on script execution — if the element is absent from the DOM it cannot run.
- The `onerror=` substring was intentionally NOT asserted to be absent from `innerHTML`, because it lawfully appears inside the escaped text string `&lt;img src=x onerror=...&gt;`. The DOM-element query (`querySelector('img')`) is the authoritative check.
- A regression to `{@html}` would fail Test 1 immediately: `querySelector('img')` would return a live element and the `&lt;img` escaped-text assertion would also likely fail (the raw `<img` would be a tag, not a text node).
