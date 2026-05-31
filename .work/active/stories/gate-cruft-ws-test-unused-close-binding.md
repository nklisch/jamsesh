---
id: gate-cruft-ws-test-unused-close-binding
kind: story
stage: review
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: cruft
created: 2026-05-31
updated: 2026-05-31
---

# Unused `close` binding in WebSocket reconnect test

## Confidence
High

## Category
unused variable

## Location
`frontend/src/lib/ws.test.ts:1154`

## Evidence
```ts
const { subscribe, close } = await import('$lib/ws.svelte');
```

`tsc --noUnusedLocals --noUnusedParameters` reports: `'close' is declared but its value is never read.`

## Removal
Remove `close` from the destructure if the test is meant to use `unsub()`, or
change the teardown step to call `close('sess-ac2')` and drop the unused `unsub`
binding if the test title is accurate.


## Implementation notes (2026-05-31)

Implemented in the consolidated v0.5.0 gate-drain pass. The pass addressed this story's release-gate finding with scoped code, generated-contract, documentation, or test updates as applicable.

## Verification (2026-05-31)

- `go test ./cmd/jamsesh/sessioncmd ./cmd/jamsesh/finalizecmd ./cmd/portal ./internal/portal/automerger ./internal/portal/sessionresume ./internal/portal/githttp ./internal/portal/playground ./internal/portal/portalinfo ./internal/db/store ./internal/portal/router` — pass.
- `npm test -- --run src/lib/screens/Login.test.ts src/lib/screens/OAuthCallback.test.ts src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/router.test.ts src/lib/portalInfo.test.ts src/lib/ws.test.ts` — pass.
- `npm run check` — 0 errors; one pre-existing `ModeSwitchDialog.svelte` warning.
- `npm run build` — pass; same pre-existing Svelte warning.
- Stale-string scans for raw-fetch/OpenAPI TODOs, EventEnvelope payload-count drift, and `git -c http.extraHeader` docs/comments passed after generated OpenAPI Go/TypeScript refresh.
