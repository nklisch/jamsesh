---
id: gate-docs-architecture-cli-resume-open-surface
kind: story
stage: done
tags: [documentation, cli]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: docs
created: 2026-05-31
updated: 2026-05-31
---

# `docs/ARCHITECTURE.md` command surface omits `--open` and `jamsesh resume`

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/ARCHITECTURE.md:130`
- Code: `cmd/jamsesh/main.go:56`

## Current doc text
> `jamsesh jam new [--org <id>] [--goal <text>] [--scope <glob>] [--mode sync|isolated] [--invite <emails>]`

## Reality
`jamsesh new` has `--open`, `jamsesh join` has `--open`, and the root command
registers `jamsesh resume`.

## Required edit
Update the slash-command subcommand list to include `--open` on create/join and
add `jamsesh resume [session-id]` with its CLI-to-browser identity handoff role.


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
