---
id: gate-docs-architecture-resume-tokens-table
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

# `docs/ARCHITECTURE.md` table list omits `resume_tokens`

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/ARCHITECTURE.md:561`
- Code: `db/schema/sqlite.sql:83`

## Current doc text
> **Tables (high-level):**

## Reality
The bundle adds a `resume_tokens` table in both SQLite and Postgres schemas,
plus a `ResumeTokenStore` interface for single-use CLI resume exchange tokens.

## Required edit
Add `resume_tokens` to the high-level table list as the hashed, single-use,
expiring token store for CLI-to-browser session resume.


## Implementation notes (2026-05-31)

Implemented in the consolidated v0.5.0 gate-drain pass. The pass addressed this story's release-gate finding with scoped code, generated-contract, documentation, or test updates as applicable.

## Verification (2026-05-31)

- `go test ./cmd/jamsesh/sessioncmd ./cmd/jamsesh/finalizecmd ./cmd/portal ./internal/portal/automerger ./internal/portal/sessionresume ./internal/portal/githttp ./internal/portal/playground ./internal/portal/portalinfo ./internal/db/store ./internal/portal/router` — pass.
- `npm test -- --run src/lib/screens/Login.test.ts src/lib/screens/OAuthCallback.test.ts src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/router.test.ts src/lib/portalInfo.test.ts src/lib/ws.test.ts` — pass.
- `npm run check` — 0 errors; one pre-existing `ModeSwitchDialog.svelte` warning.
- `npm run build` — pass; same pre-existing Svelte warning.
- Stale-string scans for raw-fetch/OpenAPI TODOs, EventEnvelope payload-count drift, and `git -c http.extraHeader` docs/comments passed after generated OpenAPI Go/TypeScript refresh.
