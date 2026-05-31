---
id: gate-cruft-oauth-stale-doc-comment-findorprovision
kind: story
stage: review
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-31
---

# Slightly stale doc-comment — references removed `FindOrProvision` instead of current `FindOrProvisionAt`

## Confidence
Low

## Category
stale comment

## Location
`internal/portal/auth/oauth.go:176-177`

## Evidence
```go
// Map the provider Identity to the shared auth.Identity type used by
// FindOrProvision.
id := Identity{ ... }
acc, _, err := FindOrProvisionAt(ctx, h.store, id, h.clock.Now())
```

## Removal
Update the comment to say `FindOrProvisionAt` (the actual callee). `FindOrProvision` is the deadcode-flagged unreachable function at `internal/portal/auth/provision.go:42` (not in this bundle) — likely scheduled for removal in a separate pass, but the doc-comment drift was introduced here.

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
