---
id: gate-tests-org-row-preventdefault
kind: story
stage: implementing
tags: [testing]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: tests
created: 2026-05-20
updated: 2026-05-20
---

# Org-row click `preventDefault` assertion missing

## Priority
Medium

## Spec reference
Item: `spa-logged-in-landing-home-screen`
Acceptance criterion: "Click events `preventDefault` and call `navigate()` to keep SPA routing for normal clicks." AC also: "each org row ... clickable (and middle-clickable via real `<a href>`)..."

## Gap type
missing test for boundary

## Suggested test
```ts
it('clicking an org row prevents default navigation (SPA-only)', async () => {
  setOrgs([
    { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
    { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
  ]);
  render(Home);
  const link = screen.getAllByRole('link')[0];
  const event = new MouseEvent('click', { bubbles: true, cancelable: true });
  link.dispatchEvent(event);
  expect(event.defaultPrevented).toBe(true);
});
```

## Test location (suggested)
`frontend/src/lib/screens/Home.test.ts`

## Context
The existing test "clicking an org row navigates via navigate() and
prevents default" asserts only that `mockNavigate` was called —
`preventDefault` is in the test name but never actually checked. If the
implementation drops `e.preventDefault()`, the test still passes because
navigate-mock fires before any default action. The boundary "real
`<a href>` works for middle-click but does not full-page-load on normal
click" is the actual spec contract; the assertion needs
`event.defaultPrevented === true`.
