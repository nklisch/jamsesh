---
id: gate-cruft-security-csp-report-placeholder
kind: story
stage: done
tags: [cleanup, documentation]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: cruft
created: 2026-05-31
updated: 2026-05-31
---

# CSP report documentation still says the wired endpoint is a placeholder

## Confidence
Medium

## Category
stale comment

## Location
`docs/SECURITY.md:256`

## Evidence
```md
**CSP regression detection:** A `Content-Security-Policy-Report-Only` header
with `report-uri /_csp-report` is emitted alongside the enforced CSP so
inline-script policy violations surface in server logs. The `/_csp-report`
route is a placeholder; see backlog item
```

`internal/portal/router/router.go:160` registers `POST /_csp-report`, and
`router.go:248` implements `cspReport`.

## Removal
Update the paragraph to describe the current unauthenticated report sink and
remove the obsolete backlog pointer.


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

Final Opus review found residual stale CSP wording. Corrected in the follow-up pass by changing `docs/SECURITY.md` to name the real `/_csp-report` public endpoint and describe it as an implemented unauthenticated structured-log sink rather than a placeholder.
