---
id: feature-auth-signout-backend-revoke
kind: feature
stage: drafting
tags: [security, portal, ui, auth, tokens]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# Sign-out backend revoke

## Brief

Today `signOut()` in `frontend/src/lib/auth.svelte.ts` clears local
tokens but never tells the backend. The access token and (more
importantly) the refresh token stay valid on the server until their
natural expiry, so a token that leaked earlier (browser extension,
shoulder-surf, malware) remains replayable through a "sign-out" event.

This feature adds a server-side revoke endpoint and wires the SPA's
sign-out flow to call it best-effort. It is cleanly scoped — one new
backend route, one DB query (mark refresh token revoked), and a
best-effort SPA call that does not block local sign-out on network
failure. Two prior autopilot triages flagged this as needing
feature-scope design rather than a single-stride story; this feature
captures that.

## Member stories

- `gate-security-signout-no-backend-revoke` —
  add `POST /api/auth/logout` (or `/api/auth/session/revoke`) that
  marks the bearer/refresh row revoked; SPA calls it before clearing
  local state, ignores failures

Likely the design pass will split this into 2-3 stories:
- backend endpoint + DB-level revoke
- frontend best-effort call + offline-safe failure
- (possibly) test coverage for the revoke path

## Approach (high level)

Feature-design will refine. Open questions for the design pass:

- Endpoint shape: `POST /api/auth/logout` (Bearer header drives which
  token to revoke) vs `/api/auth/session/revoke` with explicit body.
- Should logout also revoke the access token, or rely on its short TTL?
- Server-side state for revoked tokens — column on `oauth_tokens` or a
  separate revocation table?
