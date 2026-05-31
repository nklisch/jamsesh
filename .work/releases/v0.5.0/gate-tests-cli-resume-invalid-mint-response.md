---
id: gate-tests-cli-resume-invalid-mint-response
kind: story
stage: done
tags: [testing, cli]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: tests
created: 2026-05-31
updated: 2026-05-31
---

# CLI mint/open helper invalid responses are not tested

## Priority
Critical

## Spec reference
Item: `epic-cli-browser-session-resume-cli-handoff-mint-open-adopt`
Acceptance criterion: "Empty `ResumeUrl` / `SessionId` mismatch -> error before opening anything."

## Gap type
missing test for error case

## Suggested test
```go
// Return 200 from /api/session-resumes with empty ResumeUrl or mismatched
// SessionId; assert openSilent/openURL are not called and no token is printed.
```

## Test location (suggested)
`cmd/jamsesh/sessioncmd/resume_test.go`


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

Final Opus review found the invalid-200-body cases were not covered. Corrected in the follow-up pass with `TestResumeAction_invalidMintResponseDoesNotOpenOrLeak` in `cmd/jamsesh/sessioncmd/resume_test.go`, covering empty `resume_url` and session-id mismatch responses while asserting no browser open and no `#rt=` leak.
