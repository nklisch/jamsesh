---
id: gate-docs-security-public-surface-stale
kind: story
stage: done
tags: [documentation, security]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: docs
created: 2026-05-31
updated: 2026-05-31
---

# `docs/SECURITY.md` says default deployment has no anonymous endpoints beyond auth

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SECURITY.md:317`
- Code: `cmd/portal/main.go:1012`

## Current doc text
> The portal is designed to be safe in a hostile network with default configuration (HTTPS-only, token-authenticated, no anonymous endpoints except auth initiation).

## Reality
The current portal has intentional public endpoints beyond auth initiation,
including `/_csp-report`, `/api/portal/info`, and the new unauthenticated
`POST /api/session-resumes/exchange`; playground public endpoints are also
exposed when enabled.

## Required edit
Replace the stale sentence with a current public-surface statement that names
the intentionally public endpoints/classes and keeps the security posture
present-tense.


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

The follow-up docs correction also fixed the CSP path listed under public HTTP surfaces: `docs/SECURITY.md` now names `/_csp-report` instead of `/api/csp-report` and keeps the public-surface section present-tense.
