---
id: epic-portal-foundation-auth-flows-sender-and-magic-link
kind: story
stage: implementing
tags: [portal, security]
parent: epic-portal-foundation-auth-flows
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Auth Flows — Sender + Magic Link

## Scope

Build the email Sender abstraction with 4 concrete implementations
(SMTP, SendGrid, Postmark, Resend), the auto-provisioning helper,
and the magic-link request/exchange REST endpoints.

## Units delivered

- `internal/portal/senders/sender.go` — Sender interface +
  ErrTransient / ErrPermanent sentinels
- `internal/portal/senders/smtp.go` — wneessen/go-mail-based
- `internal/portal/senders/sendgrid.go`
- `internal/portal/senders/postmark.go`
- `internal/portal/senders/resend.go`
- `internal/portal/senders/factory.go` — `New(cfg) (Sender, error)`
- `internal/portal/config/config.go` (edit) — add `EmailConfig`
  field + env overlay + validation
- `internal/portal/auth/provision.go` — `FindOrProvision`
- `internal/portal/auth/magic_link.go` — handler logic
- `docs/openapi.yaml` (edit) — `/api/auth/magic-link/request` and `/api/auth/magic-link/exchange`
- Regenerated server.gen.go and types.gen.ts
- `cmd/portal/main.go` (edit) — wire Sender + Provider into router
- Tests for each

## Acceptance Criteria

- [ ] Each Sender implementation Round-trips: `factory.New` returns
      a working Sender for each provider value
- [ ] SMTP sender works against a local `httptest`-like mock SMTP
      (use `wneessen/go-mail`'s test server or a small custom mock)
- [ ] Hosted senders (SendGrid/Postmark/Resend) succeed against
      `httptest.NewServer` mocks with their canonical payload shape
- [ ] Error normalization: 4xx → ErrPermanent, 5xx + network →
      ErrTransient (each provider's test asserts this)
- [ ] `FindOrProvision` creates account + org + member on first
      sign-in; returns existing on second call (idempotent)
- [ ] Org slug collision handled (deterministic suffix)
- [ ] `POST /api/auth/magic-link/request` returns 204 on success;
      Sender called with the right URL
- [ ] `POST /api/auth/magic-link/exchange` returns 200 + TokenPair
      on first use; returns 401 on second use of same token
- [ ] Tokens issued via `tokens.Service.Issue` (already at done)

## Notes

- `email-senders` skill (auto-loaded) carries the verified library
  patterns; use it for SMTP/SendGrid/Postmark/Resend specifics.
- The Sender factory dispatches on `cfg.Provider` string; the
  selected provider's sub-struct must have valid fields (validation
  runs at `config.Load` time).
- Magic-link URL: `<portal_url>/auth/magic-link?token=<raw>`. The
  SPA handles the inbound link; this story's `request` endpoint
  just generates the URL and ships it via Sender.
- Auto-provisioning: see parent feature body Unit 4 for algorithm.
- The `cmd/portal/main.go` wiring adds Sender construction
  alongside the existing tokens wire.
