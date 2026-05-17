---
name: email-senders
description: Pluggable transactional email Sender abstraction for jamsesh magic-link delivery. Auto-load when implementing, modifying, or wiring the `Sender` interface or any of its concrete implementations (SMTP via wneessen/go-mail, SendGrid, Postmark, Resend). Covers idempotency, error taxonomy, version pins, and provider quirks. Triggers on `sendgrid-go`, `resend-go`, `postmark`, `net/smtp`, `wneessen/go-mail`, `Sender interface`, `magic link`, `transactional email`.
user-invocable: false
---

# Email Senders (jamsesh)

Pluggable transactional-email abstraction backing magic-link delivery
in `epic-portal-foundation-auth-flows`. Single `Sender` interface, four
concrete implementations, provider selected by config.

## Provider matrix (verified 2026-05-16)

| Provider | Library | Version | Status | Notes |
|----------|---------|---------|--------|-------|
| SMTP | `github.com/wneessen/go-mail` | v0.7.3 (2026-05-12) | Active; OpenSSF best-practices badge | Default for self-host |
| SendGrid | `github.com/sendgrid/sendgrid-go` | v3.16.1 (2025-05-29) | Maintained by Twilio | Stable but slowing — flag if it stops shipping |
| Postmark | `github.com/mrz1836/postmark` | v1.9.2 | Community fork of `keighl/postmark` | **No official Go SDK exists** — keep adapter thin |
| Resend | `github.com/resend/resend-go/v3` | v3.6.0 (2026-04-20) | Official, very active | Idempotency-Key header native, 24h window |

Do NOT switch to `net/smtp` (stdlib). It's missing context support, modern
auth methods, and TLS niceties. `wneessen/go-mail` is the 2026 idiom.

## The Sender interface

```go
// internal/email/sender.go
package email

import (
    "context"
    "errors"
)

type Sender interface {
    Send(ctx context.Context, msg Message, opts ...SendOption) (Receipt, error)
}

type Message struct {
    To      string
    From    string
    Subject string
    Text    string
    HTML    string
}

type Receipt struct {
    Provider          string // "smtp" | "sendgrid" | "postmark" | "resend"
    ProviderMessageID string // empty for SMTP
}

type sendCfg struct {
    IdempotencyKey string
    ReplyTo        string
    Tag            string
}

type SendOption func(*sendCfg)

func WithIdempotencyKey(k string) SendOption { return func(c *sendCfg) { c.IdempotencyKey = k } }
func WithReplyTo(r string) SendOption        { return func(c *sendCfg) { c.ReplyTo = r } }
func WithTag(t string) SendOption            { return func(c *sendCfg) { c.Tag = t } }

// Error taxonomy — adapters classify and wrap.
var (
    ErrTransient = errors.New("email: transient delivery failure") // caller may retry
    ErrPermanent = errors.New("email: permanent delivery failure") // do not retry (bad address etc.)
    ErrAuth      = errors.New("email: provider auth/config failure")
)
```

### Why these decisions

- **Synchronous `Send`.** Magic-link delivery only needs durable accept
  by the provider; an async callback adds complexity for no gain. The
  caller handles retry/dead-letter.
- **Functional options.** Lets each adapter use what it can support
  (Resend gets the Idempotency-Key; SMTP ignores it). New options ship
  without interface churn.
- **Three error sentinels.** Caller decides retry policy from the
  sentinel, not from provider-native error introspection.

## Provider quirks

### SMTP (`wneessen/go-mail`)

```go
import "github.com/wneessen/go-mail"

m := mail.NewMsg()
m.From(msg.From); m.To(msg.To); m.Subject(msg.Subject)
m.SetBodyString(mail.TypeTextPlain, msg.Text)
if msg.HTML != "" { m.AddAlternativeString(mail.TypeTextHTML, msg.HTML) }

c, err := mail.NewClient(host,
    mail.WithPort(587),
    mail.WithSMTPAuth(mail.SMTPAuthPlain),
    mail.WithUsername(user), mail.WithPassword(pass),
    mail.WithTLSPolicy(mail.TLSMandatory),
)
if err := c.DialAndSendWithContext(ctx, m); err != nil {
    return Receipt{}, classifySMTP(err)
}
```

- No native message ID exposed; `Receipt.ProviderMessageID` stays empty.
- No idempotency. Caller-side dedup only.
- Classify on `*textproto.Error.Code` (4xx transient, 5xx permanent;
  535 auth -> `ErrAuth`).

### SendGrid (`sendgrid-go`)

- Singleton client via `sendgrid.NewSendClient(apiKey)`.
- Build mail via `helpers/mail`; `Response.StatusCode` is your classifier
  (no typed error variants).
- `X-Message-Id` header on the response gives the provider message ID.
- No first-class idempotency; emulate caller-side.

### Postmark (`mrz1836/postmark`)

- Construct via `postmark.NewClient(serverToken, accountToken)`.
- `client.SendEmail(ctx, postmark.Email{...})` returns
  `(EmailResponse, error)`; `EmailResponse.ErrorCode` is non-zero on
  rejection (success has `0`). Don't trust the Go `error` alone —
  check `ErrorCode`.
- No idempotency. Caller-side dedup only.
- Community-maintained. Keep this adapter the thinnest of the four;
  vendor-pin and code-review every upgrade.

### Resend (`resend-go/v3`)

```go
import "github.com/resend/resend-go/v3"

client := resend.NewClient(apiKey)
req := &resend.SendEmailRequest{
    From: msg.From, To: []string{msg.To}, Subject: msg.Subject,
    Text: msg.Text, Html: msg.HTML,
}
if cfg.IdempotencyKey != "" {
    req.Headers = map[string]string{"Idempotency-Key": cfg.IdempotencyKey}
}
sent, err := client.Emails.SendWithContext(ctx, req)
```

- Use `SendWithContext` (NOT plain `Send`) for cancellation.
- Native `Idempotency-Key` header, 24-hour dedup window. Format keys as
  `magic-link/<token-id>` so they're stable per logical send.
- Errors expose `*resend.Error` with `StatusCode`; classify on HTTP
  status (see `references/error-tables.md`).

## Common pitfalls

- **`*sendgrid.Response` is not an error.** `Send` returns `(*Response, error)`;
  HTTP 4xx/5xx come back with `err == nil` and a non-2xx
  `Response.StatusCode`. Always inspect `StatusCode` even when `err == nil`.
- **Postmark `ErrorCode` shadowing.** `mrz1836/postmark` returns
  `(EmailResponse, nil)` even when Postmark rejected the message;
  the `ErrorCode` field is the real signal.
- **Resend `Send` vs `SendWithContext`.** Plain `Send` ignores context
  cancellation. Always use the context variant in jamsesh.
- **SMTP STARTTLS misconfig.** Default to `mail.TLSMandatory` unless
  the operator config explicitly opts out. Plaintext SMTP for
  magic-links is a credential-leak hazard.
- **Idempotency key reuse across logical sends.** Resend dedup is
  24-hour. Reusing a key for a different message returns the *original*
  message-id — surprising behavior if the caller doesn't expect it.
  Always derive the key from the magic-link token (or other logical
  send id), not from the recipient or timestamp.
- **From-address validation.** SendGrid and Resend require verified
  sender domains; Postmark requires signature confirmation. Surface
  config-time validation, not runtime errors.

## jamsesh-specific decisions

- **One config key per deploy** selects the provider (`email.provider:
  smtp|sendgrid|postmark|resend`); per-provider credentials live under
  `email.<provider>.*`. SMTP is the self-host default.
- **One file per adapter** under `internal/email/`: `smtp.go`,
  `sendgrid.go`, `postmark.go`, `resend.go`. All implement `Sender`.
- **Caller is the auth flow.** `epic-portal-foundation-auth-flows`
  derives the idempotency key from `magic_link_tokens.id` and applies
  retry-on-`ErrTransient` with capped exponential backoff.
- **Postmark is the weakest link.** Community SDK + Postmark's
  ActiveCampaign acquisition increase abstraction value. Keep the
  Postmark adapter under 100 lines so a future swap is cheap.

## Reference

See `references/error-tables.md` for per-provider HTTP/SMTP status
classification and `references/config-shape.md` for the YAML config
schema each adapter consumes.
