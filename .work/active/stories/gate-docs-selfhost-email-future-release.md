---
id: gate-docs-selfhost-email-future-release
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

# SELF_HOST.md §6 Email section gated as "future release" even though sender providers have shipped

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SELF_HOST.md:330-339`
- Code: `internal/portal/config/config.go:622-625`;
  `internal/portal/senders/` contains real SMTP/SendGrid/Postmark/Resend
  implementations; `compose.yaml:29` already passes
  `JAMSESH_EMAIL_FROM`

## Current doc text
> > **NOTE:** Email provider configuration lands with
> > `epic-portal-foundation-auth-flows` in a future release. This section
> > describes the expected provider options.
> The portal supports magic-link email auth … Provider options will
> include SMTP (self-host default), SendGrid, Postmark, and Resend.
> Per-provider env var configuration will be documented in the
> auth-flows release notes.

## Reality
Email senders ship in v0.1.0 (`internal/portal/senders/`). Primary env
vars `JAMSESH_EMAIL_PROVIDER`, `JAMSESH_EMAIL_FROM`, plus per-provider
settings (SMTP host/port/user/pass, SendGrid API key, Postmark server
token, Resend API key) and `_FILE` companions are live and partially
documented in §2.

## Required edit
Remove the "future release" NOTE. Document the primary email env vars
(`JAMSESH_EMAIL_PROVIDER` with values, `JAMSESH_EMAIL_FROM`,
per-provider host/port/credential variables) in §6 to match what §2
already references for the `_FILE` companions.
