---
id: gate-cruft-errnotfound-unused-import-suppressions
kind: story
stage: done
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: cruft
created: 2026-05-31
updated: 2026-05-31
---

# Automerger ErrNotFound test keeps unused imports alive with dummy vars

## Confidence
Medium

## Category
unused import

## Location
`internal/portal/automerger/errnotfound_test.go:245`

## Evidence
```go
// Suppress unused imports.
var _ *gogit.Repository
var _ *object.Commit
var _ plumbing.Hash
```

## Removal
Remove the suppression block; remove the unused `gogit` and `object` imports.
Keep `plumbing`, which is used earlier in the file.


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

## Completion correction (2026-05-31)

Final Opus review found the earlier consolidated implementation note was too broad. Corrected in the follow-up pass by removing the dummy suppression block and the now-unused `gogit` and `object` imports from `internal/portal/automerger/errnotfound_test.go`; `plumbing` remains because the file uses it directly.
