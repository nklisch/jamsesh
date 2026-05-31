---
id: gate-tests-spa-resume-public-routes
kind: story
stage: implementing
tags: [testing, ui]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: tests
created: 2026-05-31
updated: 2026-05-31
---

# SPA resume routes are not tested as public router entries

## Priority
Critical

## Spec reference
Item: `epic-cli-browser-session-resume-spa-route-route-screen`
Acceptance criterion: "Public route flags; nav paths derived from the response."

## Gap type
missing test for route/state partition

## Suggested test
```ts
// Match /playground/s/:id/resume and /orgs/:org/sessions/:id/resume;
// assert requiresAuth === false and the specific resume route wins.
```

## Test location (suggested)
`frontend/src/lib/router.test.ts`

