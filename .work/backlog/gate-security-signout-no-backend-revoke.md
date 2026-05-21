---
id: gate-security-signout-no-backend-revoke
kind: story
stage: drafting
tags: [security]
parent: null
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-20
updated: 2026-05-20
---

# signOut() clears local tokens without notifying backend to revoke them

## Severity
Low

## Domain
Authentication & Authorization

## Location
`frontend/src/lib/auth.svelte.ts:53-62`

## Evidence
```ts
signOut(): void {
  _token = null;
  _refresh = null;
  _currentUser = null;
  _orgs = null;
  _loadingMe = null;
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_KEY);
  navigate('/login');
},
```

## Remediation direction
After sign-out, both the access token and (more importantly) the refresh
token remain valid on the server until their natural expiry. A token
that leaked earlier (browser extension, shared-device shoulder-surf of
localStorage, malware) is not invalidated by the user clicking "Sign
out".

Add a best-effort `POST /api/auth/logout` (or
`/api/auth/session/revoke`) call before clearing local state so the
server can mark the refresh token revoked; ignore failures so offline
sign-out still works locally.

## Autopilot deferral note (2026-05-20)

Deferred from `release_binding: v0.3.0` by `/agile-workflow:autopilot --all`.
Rationale: requires a new backend endpoint (`POST /api/auth/logout` with
DB-level refresh-token revocation) plus the frontend best-effort call —
this is feature-scope work, not a single-stride story. Moved to backlog
for proper scoping in a future release. Per release-v0.3.0 file's
documented escape hatch.
