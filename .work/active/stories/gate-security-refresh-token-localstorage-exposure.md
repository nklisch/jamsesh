---
id: gate-security-refresh-token-localstorage-exposure
kind: story
stage: done
tags: [security]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: security
created: 2026-05-20
updated: 2026-05-31
---

# Refresh token persisted in localStorage, exposed to any XSS

## Severity
Medium

## Domain
Data Protection (token storage)

## Location
`frontend/src/lib/auth.svelte.ts:49-50`

## Evidence
```ts
localStorage.setItem(TOKEN_KEY, access);
localStorage.setItem(REFRESH_KEY, refreshTok);
```

## Remediation direction
Refresh tokens are long-lived credentials. Storing them in `localStorage`
makes them readable by any script with execution context, so any single
XSS — including from a future third-party dep, a Markdown render path, or
a user-controlled-string sink — exfiltrates a refresh token that can be
replayed indefinitely until the backend rotates/revokes it.

Industry guidance (OWASP, IETF OAuth 2.0 for Browser-Based Apps BCP) is
to keep refresh tokens out of JS-accessible storage entirely: deliver
them via an HttpOnly, Secure, SameSite cookie scoped to the refresh
endpoint, or use a Backend-for-Frontend that holds the refresh token
server-side and hands the SPA only short-lived access tokens.

The access token is also exposed but its blast radius is bounded by TTL;
the refresh token is the load-bearing concern.

## Autopilot deferral note (2026-05-20)

Deferred from `release_binding: v0.3.0` by `/agile-workflow:autopilot --all`.
Rationale: this is genuinely architectural — the remediation requires
either (a) HttpOnly cookie + backend endpoint changes to set/read the
cookie + refresh-endpoint scoping, or (b) a Backend-for-Frontend pattern
where the SPA receives only short-lived access tokens. Either path is
feature-scope (probably epic-scope) work requiring proper design and
cross-stack implementation, not a single-stride story. Moved to backlog
for scoping via `/agile-workflow:scope` in a future release. Per
release-v0.3.0 file's documented escape hatch.

## Autopilot triage (2026-05-24)

Left at drafting. Per the prior autopilot deferral note: genuinely
architectural — remediation requires either HttpOnly cookie + backend
changes, or a Backend-for-Frontend pattern. Either path is
feature-scope (probably epic-scope) work. Awaiting human
`/agile-workflow:scope` for proper design and cross-stack
implementation.

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
