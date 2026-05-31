---
id: gate-security-oauth-state-no-client-binding
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

# OAuth state nonce held only by backend; client has no tab-binding

## Severity
Low

## Domain
Authentication & Authorization

## Location
`frontend/src/lib/screens/Login.svelte:60-65`, `frontend/src/lib/screens/OAuthCallback.svelte:33-43`

## Evidence
```ts
sessionStorage.setItem('oauth.provider', 'github');
if (returnTo) {
  sessionStorage.setItem('oauth.return_to', returnTo);
} else {
  sessionStorage.removeItem('oauth.return_to');
}
```

## Remediation direction
The client persists only `oauth.provider` and `oauth.return_to` in
sessionStorage; the CSRF-defeating `state` nonce is held entirely
server-side and the client doesn't keep its own copy to cross-check.

If the callback ever runs in a tab/session different from the one that
initiated the flow (login-CSRF where an attacker tricks a victim into
completing an attacker-initiated OAuth login), the client cannot detect
the mismatch — it relies fully on the backend's state-binding.

Defense-in-depth: at OAuth start, persist a fresh client-side
correlation id (random UUID) in sessionStorage; have the backend echo
the same id into the callback (or include it in the authorize-url
`state`); at callback, assert the values match before posting to
`/api/auth/oauth/callback`. Reject otherwise.

## Autopilot deferral note (2026-05-20)

Deferred from `release_binding: v0.3.0` by `/agile-workflow:autopilot --all`.
Rationale: this is cross-stack (frontend correlation-id storage + backend
state echo) and needs feature-scope design before implementation — it's
larger than a single-stride story. Moved to backlog for proper scoping in
a future release. Per release-v0.3.0 file's documented escape hatch:
"clear `release_binding` to defer to a later release."

## Autopilot triage (2026-05-24)

Left at drafting. The body already carries an "Autopilot deferral
note" from 2026-05-20 explaining this is cross-stack
(frontend correlation-id storage + backend state echo) and needs
feature-scope design before implementation. Respecting that prior
triage; this item is awaiting human `/agile-workflow:scope` to
promote into a properly-designed feature.

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
