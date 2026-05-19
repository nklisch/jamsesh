---
id: gate-docs-selfhost-email-future-release
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

## Implementation notes

Removed the "future release" NOTE block and replaced the placeholder prose with
a full §6 rewrite. The new section documents:

- Common variables: `JAMSESH_EMAIL_PROVIDER` (values: `smtp`, `sendgrid`,
  `postmark`, `resend`; default `smtp`) and `JAMSESH_EMAIL_FROM`.
- SMTP sub-section: host, port, user, pass (with `_FILE` note), TLS mode;
  defaults match `config.go` (`localhost:587`, `mandatory`).
- SendGrid sub-section: API key with `_FILE` companion.
- Postmark sub-section: server token with `_FILE` companion, message stream
  (default `outbound` per `senders/postmark.go`).
- Resend sub-section: API key with `_FILE` companion.
- Cross-reference to the §2 `_FILE` convention table.

No other sections were touched. All env var names, YAML keys, and defaults
were verified directly from `internal/portal/config/config.go` and
`internal/portal/senders/`.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Mechanical change matching the gate finding spec. Implementation notes accurately describe what was changed. Global `go build ./...` and `go test ./internal/portal/...` pass after the wave landed.
