---
id: gate-docs-security-public-surface-stale
kind: story
stage: implementing
tags: [documentation, security]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: docs
created: 2026-05-31
updated: 2026-05-31
---

# `docs/SECURITY.md` says default deployment has no anonymous endpoints beyond auth

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SECURITY.md:317`
- Code: `cmd/portal/main.go:1012`

## Current doc text
> The portal is designed to be safe in a hostile network with default configuration (HTTPS-only, token-authenticated, no anonymous endpoints except auth initiation).

## Reality
The current portal has intentional public endpoints beyond auth initiation,
including `/_csp-report`, `/api/portal/info`, and the new unauthenticated
`POST /api/session-resumes/exchange`; playground public endpoints are also
exposed when enabled.

## Required edit
Replace the stale sentence with a current public-surface statement that names
the intentionally public endpoints/classes and keeps the security posture
present-tense.

