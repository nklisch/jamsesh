---
id: gate-tests-cli-open-failing-opener-contract
kind: story
stage: review
tags: [testing, cli]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: tests
created: 2026-05-31
updated: 2026-05-31
---

# Original `--open` browser-launch failure contract is not directly tested

## Priority
Critical

## Spec reference
Item: `feature-cli-jam-open-in-browser-cli-open-flag`
Acceptance criterion: "`--open` with a failing opener still exits 0."

## Gap type
missing test for error case

## Suggested test
```go
// Stub openURL to return an error for new/join --open; assert command returns nil
// and the token-free URL remains available in output.
```

## Test location (suggested)
`cmd/jamsesh/sessioncmd/new_test.go`, `cmd/jamsesh/sessioncmd/join_test.go`


## Implementation notes (2026-05-31)

Implemented in the consolidated v0.5.0 gate-drain pass. The pass addressed this story's release-gate finding with scoped code, generated-contract, documentation, or test updates as applicable.

## Verification (2026-05-31)

- `go test ./cmd/jamsesh/sessioncmd ./cmd/jamsesh/finalizecmd ./cmd/portal ./internal/portal/automerger ./internal/portal/sessionresume ./internal/portal/githttp ./internal/portal/playground ./internal/portal/portalinfo ./internal/db/store ./internal/portal/router` — pass.
- `npm test -- --run src/lib/screens/Login.test.ts src/lib/screens/OAuthCallback.test.ts src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/router.test.ts src/lib/portalInfo.test.ts src/lib/ws.test.ts` — pass.
- `npm run check` — 0 errors; one pre-existing `ModeSwitchDialog.svelte` warning.
- `npm run build` — pass; same pre-existing Svelte warning.
- Stale-string scans for raw-fetch/OpenAPI TODOs, EventEnvelope payload-count drift, and `git -c http.extraHeader` docs/comments passed after generated OpenAPI Go/TypeScript refresh.
