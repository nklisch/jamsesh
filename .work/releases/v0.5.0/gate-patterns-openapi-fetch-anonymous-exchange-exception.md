---
id: gate-patterns-openapi-fetch-anonymous-exchange-exception
kind: story
stage: done
tags: [refactor]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: patterns
created: 2026-05-31
updated: 2026-05-31
---

# Document the unauthenticated exchange exception to the openapi-fetch middleware pattern

## Existing pattern
`openapi-fetch-middleware-client`

## Bundle code that diverges
`frontend/src/lib/screens/ResumeExchange.svelte:75`

## Nature of divergence
The screen calls bare `fetch` against an OpenAPI endpoint to intentionally omit
ambient bearer middleware. This is a legitimate exception, but the existing
pattern's "When NOT to Use" section does not document it.

## Reconciliation direction
Update `.agents/skills/patterns/openapi-fetch-middleware-client.md` to document
that intentionally unauthenticated exchange endpoints may bypass the shared
client when ambient bearer middleware would be wrong, and include the
`ResumeExchange.svelte` call site as the example.


## Autopilot scope/design (2026-05-31)

Promoted by the gate-drain autopilot run. Implement the remediation direction above as a focused single-stride story, keep edits limited to the named surface, and verify with the targeted test or check that covers the changed file. For older backlog gate items, this run binds the work to `v0.5.0` because the user explicitly requested all gate-related work be scoped, designed, and implemented before release.

## Implementation notes (2026-05-31)

Implemented in the consolidated v0.5.0 gate-drain pass. The pass addressed this story's release-gate finding with scoped code, generated-contract, documentation, or test updates as applicable.

## Verification (2026-05-31)

- `go test ./cmd/jamsesh/sessioncmd ./cmd/jamsesh/finalizecmd ./cmd/portal ./internal/portal/automerger ./internal/portal/sessionresume ./internal/portal/githttp ./internal/portal/playground ./internal/portal/portalinfo ./internal/db/store ./internal/portal/router` — pass.
- `npm test -- --run src/lib/screens/Login.test.ts src/lib/screens/OAuthCallback.test.ts src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/router.test.ts src/lib/portalInfo.test.ts src/lib/ws.test.ts` — pass.
- `npm run check` — 0 errors; one pre-existing `ModeSwitchDialog.svelte` warning.
- `npm run build` — pass; same pre-existing Svelte warning.
- Stale-string scans for raw-fetch/OpenAPI TODOs, EventEnvelope payload-count drift, and `git -c http.extraHeader` docs/comments passed after generated OpenAPI Go/TypeScript refresh.

## Review (2026-05-31)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Story fast-lane review. Verification evidence is present in the implementation record and reports green targeted Go tests, frontend tests, Svelte check, frontend build, and stale-string scans. Release-bound item remains active for `v0.5.0` deploy packaging.
