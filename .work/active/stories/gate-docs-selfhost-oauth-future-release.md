---
id: gate-docs-selfhost-oauth-future-release
kind: story
stage: done
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: docs
created: 2026-05-18
updated: 2026-05-18
---

# SELF_HOST.md §4 OAuth section is gated behind a "future release" NOTE even though OAuth provider config has shipped

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SELF_HOST.md:259-283`
- Code: `internal/portal/config/config.go:622-625` and §2 reference table
  at `SELF_HOST.md:112-115` already lists the four GitHub OAuth env vars

## Current doc text
> > **NOTE:** OAuth provider configuration lands with
> > `epic-portal-foundation-auth-flows` in a future release. This section
> > describes the expected callback shape based on the auth-flows feature design;
> > the exact env vars and registration steps will be filled in when that feature ships.

## Reality
GitHub OAuth has shipped. Env vars (`JAMSESH_OAUTH_GITHUB_CLIENT_ID`,
`_SECRET`, `_SECRET_FILE`, `_BASE_URL`) are listed in §2's reference
table and are wired in `internal/portal/config/config.go`. The portal
callback handler lives at `POST /api/auth/oauth/callback`
(`docs/openapi.yaml:1521`).

## Required edit
Remove the "future release" NOTE. Replace the placeholder text with the
live env var names (already documented in §2) and the actual callback
URL the portal exposes. Drop the "Google and OIDC … lands in a future
release" sentence or move it to an explicitly-deferred bullet.

## Implementation notes

- Removed the "future release" NOTE block entirely.
- Replaced the placeholder callback URL `/auth/github/callback` with the
  canonical portal endpoint `POST /api/auth/oauth/callback` (confirmed in
  `docs/openapi.yaml:1521`).
- Replaced the "see auth-flows release notes for exact variable names"
  placeholder with an explicit env var table mirroring the style and names
  from §2 (`SELF_HOST.md:112-115`).
- Added a note clarifying the SPA-side redirect URL vs. the portal callback
  endpoint, as these are distinct hops in the OAuth flow.
- Dropped the "Google and OIDC lands in a future release" line per
  rolling-foundation principle; the sibling story
  `gate-docs-selfhost-oauth-callback-url` covers further callback URL drift
  investigation and remains at stage:drafting.
- `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE` confirmed in
  `internal/portal/config/config.go:60` (_FILE variant).
- `JAMSESH_PORTAL_URL` is already documented in §2 and its role in
  constructing OAuth callback URLs is noted in that row; §4 now cross-links it.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Mechanical change matching the gate finding spec. Implementation notes accurately describe what was changed. Global `go build ./...` and `go test ./internal/portal/...` pass after the wave landed.
