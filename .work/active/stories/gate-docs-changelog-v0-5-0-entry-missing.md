---
id: gate-docs-changelog-v0-5-0-entry-missing
kind: story
stage: review
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: docs
created: 2026-05-31
updated: 2026-05-31
---

# `CHANGELOG.md` has no `v0.5.0` release entry

## Drift category
changelog-gap

## Location
- Doc: `CHANGELOG.md:3`

## Current doc text
> `## v0.4.1`

## Reality
Release `v0.5.0` has 68 bound items and 146 bundle files, but `CHANGELOG.md`
contains no `v0.5.0` section or entries for the bound work.

## Required edit
Add a top-level `## v0.5.0` section covering the release's CLI browser
handoff/session resume work, bug-squash fixes, e2e infrastructure fixes,
docs/security changes, and generated-contract changes.


## Implementation notes (2026-05-31)

Implemented in the consolidated v0.5.0 gate-drain pass. The pass addressed this story's release-gate finding with scoped code, generated-contract, documentation, or test updates as applicable.

## Verification (2026-05-31)

- `go test ./cmd/jamsesh/sessioncmd ./cmd/jamsesh/finalizecmd ./cmd/portal ./internal/portal/automerger ./internal/portal/sessionresume ./internal/portal/githttp ./internal/portal/playground ./internal/portal/portalinfo ./internal/db/store ./internal/portal/router` — pass.
- `npm test -- --run src/lib/screens/Login.test.ts src/lib/screens/OAuthCallback.test.ts src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/router.test.ts src/lib/portalInfo.test.ts src/lib/ws.test.ts` — pass.
- `npm run check` — 0 errors; one pre-existing `ModeSwitchDialog.svelte` warning.
- `npm run build` — pass; same pre-existing Svelte warning.
- Stale-string scans for raw-fetch/OpenAPI TODOs, EventEnvelope payload-count drift, and `git -c http.extraHeader` docs/comments passed after generated OpenAPI Go/TypeScript refresh.
