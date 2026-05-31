---
id: gate-cruft-per-package-stores-wrapper-helpers
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

# Per-package one-line wrapper `stores()` duplicated across test packages

## Confidence
Low

## Category
single-use helper

## Location
`internal/db/store/helpers_test.go:31-34` (and similar shape in `internal/portal/playground/provision_test.go`)

## Evidence
```go
// stores is a one-line wrapper over storetest.Stores so existing call sites
// in this package don't have to spell the package-qualified name each time.
func stores(t *testing.T) []storetest.DialectHarness {
    t.Helper()
    return storetest.Stores(t)
}
```

## Removal
The wrapper exists only to save typing `storetest.` at call sites. Inline `storetest.Stores(t)` at the (few) call sites in each test file and remove both wrappers + their comment blocks. Note: this is contested — some projects deliberately keep such shortcuts. Low confidence; treat as judgment.

## Autopilot triage (2026-05-24)

Left at drafting. The body explicitly flags this as a contested
judgment call: "this is contested — some projects deliberately keep
such shortcuts. Low confidence; treat as judgment." Autopilot
declines to autonomously make this style call; awaiting human
decision on whether to inline `storetest.Stores(t)` at call sites or
keep the per-package shortcuts.

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
