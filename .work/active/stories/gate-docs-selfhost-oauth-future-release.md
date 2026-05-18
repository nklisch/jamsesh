---
id: gate-docs-selfhost-oauth-future-release
kind: story
stage: implementing
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
