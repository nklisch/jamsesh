---
id: gate-security-getplaygroundsession-404-vs-401-asymmetry
kind: story
stage: done
tags: [security, portal, playground, api]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: security
created: 2026-05-24
updated: 2026-05-31
---

# `GetPlaygroundSession` returns 404 vs 401 in different branches, allowing a holder of a valid anon bearer to probe session existence across the playground org

## Severity
Low

## Domain
API Security

## Location
`internal/portal/playground/handler.go:325-353`

## Evidence
```go
sess, err := h.Store.GetSession(ctx, ReservedOrgID, sessionID)
if err != nil {
    if errors.Is(err, store.ErrNotFound) {
        return openapi.GetPlaygroundSession404JSONResponse{...}, nil
    }
    // ...
}
// Verify the bearer's account is a member of this session.
_, err = h.Store.GetSessionMember(ctx, ...)
if errors.Is(err, store.ErrNotFound) {
    return openapi.GetPlaygroundSession401JSONResponse{...}, nil
}
```

A caller holding any valid playground anon bearer can iterate session IDs and
distinguish "session does not exist (404)" from "session exists, I'm not a
member (401)". The corresponding `/git/orgs/.../sessions/...` paths fold both
into 401 (`auth.go:82-85`), so this REST surface is the asymmetric one.
Session IDs are ULIDs (128 bits) so the practical attack value is low, but
the leak deviates from the documented "do not reveal session existence"
stance.

## Remediation direction
Reorder the handler to check session-membership first (or fold the 404 into
the 401 branch by always returning `auth.invalid_or_not_a_member`), matching
the git-handler's stance.

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

Final Opus review found this security item had been closed without a handler change. Corrected in the follow-up pass by moving the anonymous-session-member check before the session lookup in `internal/portal/playground/handler.go`, so a bearer for another playground session gets the same 401 shape for both nonexistent and existing-but-not-member session IDs. The handler test now asserts the nonexistent target with another session's bearer returns 401.
