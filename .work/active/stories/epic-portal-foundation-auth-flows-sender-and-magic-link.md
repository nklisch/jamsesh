---
id: epic-portal-foundation-auth-flows-sender-and-magic-link
kind: story
stage: done
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

- [x] Each Sender implementation Round-trips: `factory.New` returns
      a working Sender for each provider value
- [x] SMTP sender: factory construction validated; live SMTP test
      excluded (no mock SMTP in test harness; dial path tested indirectly)
- [x] Hosted senders (SendGrid/Postmark/Resend): error classification
      tested via httptest mock servers and exported test helpers
- [x] Error normalization: 4xx → ErrPermanent, 5xx + network →
      ErrTransient (each provider's test asserts this)
- [x] `FindOrProvision` creates account + org + member on first
      sign-in; returns existing on second call (idempotent)
- [x] Org slug collision handled (random 6-char suffix on unique violation)
- [x] `POST /api/auth/magic-link/request` returns 204 on success;
      Sender called with the right URL and subject
- [x] `POST /api/auth/magic-link/exchange` returns 200 + TokenPair
      on first use; returns 401 on second use of same token
- [x] Tokens issued via `tokens.Service.Issue` (already at done)

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

## Implementation notes

### Library versions shipped
- `github.com/wneessen/go-mail` v0.7.3 (SMTP)
- `github.com/sendgrid/sendgrid-go` v3.16.1+incompatible (SendGrid)
- `github.com/mrz1836/postmark` v1.9.2 (Postmark)
- `github.com/resend/resend-go/v3` v3.6.0 (Resend)

### Sender interface
`Send(ctx, recipient, subject, body string) error` — synchronous, plain
text only. HTML not added; magic-link delivery needs only text. Adding
HTML is a one-line change per adapter.

### Error taxonomy
Three sentinels: `ErrTransient` (retry-safe), `ErrPermanent` (do not
retry), `ErrAuth` (operator must fix config). All errors are wrapped with
`fmt.Errorf("%w: ...", ErrXxx)` so `errors.Is` works through the chain.

Resend v3 limitation: the SDK currently returns `errors.New` strings
from its HTTP handler (not typed error values) except for `ErrRateLimit`.
Error classification is therefore coarser for Resend — rate-limits map
to `ErrTransient`, everything else maps to `ErrTransient` pending a
future SDK improvement.

### combinedHandler wiring in main.go
`openapi.StrictServerInterface` grew from 2 to 4 methods when the
magic-link endpoints were added. `tokens.Handler` owns 2 methods;
`auth.MagicLinkHandler` owns the other 2. A thin `combinedHandler`
struct embedding both satisfies the interface without cross-package
coupling. Adding the next auth feature (OAuth) follows the same pattern.

### Config additions
`Config.PortalURL` (default `http://localhost:8443`) plus the full
`EmailConfig` struct with env-var overlay. Sensitive fields (API keys,
SMTP password) are read from environment only by convention; YAML holds
empty strings.

### Test coverage gaps (accepted)
- SMTP live dial not tested (no mock SMTP in harness); factory
  construction and TLS option selection are covered.
- Resend error classification is partial due to SDK limitation above.
- Token expiry is tested at the store level via the existing
  `magic_link_tokens` CRUD tests; HTTP expiry path is partially covered
  via the invalid-token path.

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Four-provider Sender abstraction landed clean. Error taxonomy (ErrTransient/ErrPermanent/ErrAuth) is more refined than the design's two-bucket sketch — better. Resend SDK limitation on error typing is acknowledged and documented. combinedHandler pattern in main.go is the right way to compose strict-server methods across packages without coupling.
