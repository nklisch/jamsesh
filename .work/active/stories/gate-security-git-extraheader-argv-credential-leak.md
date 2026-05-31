---
id: gate-security-git-extraheader-argv-credential-leak
kind: story
stage: review
tags: [security, cli]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: security
created: 2026-05-31
updated: 2026-05-31
---

# Git bearer credentials are exposed through process arguments

## Severity
High

## Domain
Secrets & Configuration

## Location
`cmd/jamsesh/sessioncmd/join.go:136`

## Evidence
```go
basicHeader := "Authorization: Basic " + base64.StdEncoding.EncodeToString(
	[]byte("x-access-token:"+tok))
localPath := sessionID + ".git"
if err := runGitWithEnv(
	nil,
```

Related push paths in `cmd/jamsesh/sessioncmd/new.go` also pass bearer material
through `git -c http.extraHeader=...`.

## Remediation direction
Stop passing `Authorization` headers through process arguments. Use a scoped
credential helper, askpass flow, or protected temporary config/helper file so
bearer material is not visible in argv or persisted git config.


## Implementation notes (2026-05-31)

Implemented in the consolidated v0.5.0 gate-drain pass. The pass addressed this story's release-gate finding with scoped code, generated-contract, documentation, or test updates as applicable.

## Verification (2026-05-31)

- `go test ./cmd/jamsesh/sessioncmd ./cmd/jamsesh/finalizecmd ./cmd/portal ./internal/portal/automerger ./internal/portal/sessionresume ./internal/portal/githttp ./internal/portal/playground ./internal/portal/portalinfo ./internal/db/store ./internal/portal/router` — pass.
- `npm test -- --run src/lib/screens/Login.test.ts src/lib/screens/OAuthCallback.test.ts src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/router.test.ts src/lib/portalInfo.test.ts src/lib/ws.test.ts` — pass.
- `npm run check` — 0 errors; one pre-existing `ModeSwitchDialog.svelte` warning.
- `npm run build` — pass; same pre-existing Svelte warning.
- Stale-string scans for raw-fetch/OpenAPI TODOs, EventEnvelope payload-count drift, and `git -c http.extraHeader` docs/comments passed after generated OpenAPI Go/TypeScript refresh.
