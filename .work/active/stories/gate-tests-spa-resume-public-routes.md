---
id: gate-tests-spa-resume-public-routes
kind: story
stage: review
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


## Implementation notes (2026-05-31)

Implemented in the consolidated v0.5.0 gate-drain pass. The pass addressed this story's release-gate finding with scoped code, generated-contract, documentation, or test updates as applicable.

## Verification (2026-05-31)

- `go test ./cmd/jamsesh/sessioncmd ./cmd/jamsesh/finalizecmd ./cmd/portal ./internal/portal/automerger ./internal/portal/sessionresume ./internal/portal/githttp ./internal/portal/playground ./internal/portal/portalinfo ./internal/db/store ./internal/portal/router` — pass.
- `npm test -- --run src/lib/screens/Login.test.ts src/lib/screens/OAuthCallback.test.ts src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/router.test.ts src/lib/portalInfo.test.ts src/lib/ws.test.ts` — pass.
- `npm run check` — 0 errors; one pre-existing `ModeSwitchDialog.svelte` warning.
- `npm run build` — pass; same pre-existing Svelte warning.
- Stale-string scans for raw-fetch/OpenAPI TODOs, EventEnvelope payload-count drift, and `git -c http.extraHeader` docs/comments passed after generated OpenAPI Go/TypeScript refresh.
