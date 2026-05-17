# Per-provider error classification tables

How each adapter maps provider-native errors to jamsesh sentinels
(`ErrTransient` / `ErrPermanent` / `ErrAuth`).

## SMTP (`wneessen/go-mail`)

Error type: usually wraps `*textproto.Error` from `net/textproto`.

| SMTP status | Sentinel | Notes |
|-------------|----------|-------|
| 421 (service not available) | `ErrTransient` | Server busy / shutting down |
| 450 (mailbox busy) | `ErrTransient` | Try later |
| 451 (local error) | `ErrTransient` | Server-side glitch |
| 452 (insufficient storage) | `ErrTransient` | Mail queue full |
| 4xx (other) | `ErrTransient` | Default transient class |
| 535 (auth invalid) | `ErrAuth` | Credentials wrong |
| 550 (mailbox unavailable) | `ErrPermanent` | Address does not exist |
| 551 (user not local) | `ErrPermanent` | Routing failure |
| 552 (exceeded storage) | `ErrPermanent` | Recipient over quota — treat as permanent for magic-link |
| 553 (mailbox name invalid) | `ErrPermanent` | Bad address syntax |
| 554 (transaction failed) | `ErrPermanent` | Spam / policy rejection |
| 5xx (other) | `ErrPermanent` | Default permanent class |
| Connection / DNS / TLS errors | `ErrTransient` | Retryable |

Classifier sketch:

```go
func classifySMTP(err error) error {
    var te *textproto.Error
    if errors.As(err, &te) {
        switch {
        case te.Code == 535:
            return fmt.Errorf("%w: %v", ErrAuth, err)
        case te.Code >= 400 && te.Code < 500:
            return fmt.Errorf("%w: %v", ErrTransient, err)
        case te.Code >= 500:
            return fmt.Errorf("%w: %v", ErrPermanent, err)
        }
    }
    return fmt.Errorf("%w: %v", ErrTransient, err) // network etc.
}
```

## SendGrid (`sendgrid-go`)

`Send` returns `(*rest.Response, error)`. **Inspect
`Response.StatusCode` even when `err == nil`.**

| HTTP status | Sentinel | Notes |
|-------------|----------|-------|
| 202 | (success) | The only success code for v3 mail send |
| 400 | `ErrPermanent` | Validation failure — bad request shape |
| 401 / 403 | `ErrAuth` | API key invalid or missing scope |
| 413 | `ErrPermanent` | Payload too large |
| 429 | `ErrTransient` | Rate limited — honor `X-RateLimit-Reset` |
| 5xx | `ErrTransient` | Server problem |

## Postmark (`mrz1836/postmark`)

`SendEmail` returns `(EmailResponse, error)`. **Check `EmailResponse.ErrorCode`
in addition to the Go error** — Postmark sometimes returns a 200 OK with
a non-zero `ErrorCode`.

Selected `ErrorCode` values:

| ErrorCode | Sentinel | Meaning |
|-----------|----------|---------|
| 0 | (success) | Sent |
| 10 (bad/missing API token) | `ErrAuth` | |
| 100 (maintenance) | `ErrTransient` | |
| 300 (invalid email request) | `ErrPermanent` | |
| 400 (sender signature not confirmed) | `ErrAuth` | Config problem, not transient |
| 405 (account disabled) | `ErrAuth` | |
| 406 (inactive recipient) | `ErrPermanent` | Hard-bounce or unsubscribed |
| 412 (incompatible JSON) | `ErrPermanent` | |
| 422 (bad message stream) | `ErrPermanent` | |
| 429 (rate limited) | `ErrTransient` | |
| 500 (internal server error) | `ErrTransient` | |

Full list: https://postmarkapp.com/developer/api/overview#error-codes

## Resend (`resend-go/v3`)

Errors wrap `*resend.Error` with `StatusCode`.

| HTTP status | Sentinel | Notes |
|-------------|----------|-------|
| 200 | (success) | `sent.Id` populated |
| 400 | `ErrPermanent` | Validation failed |
| 401 | `ErrAuth` | Bad / missing API key |
| 403 | `ErrAuth` | Insufficient scope |
| 404 | `ErrPermanent` | Resource (sender domain etc.) missing |
| 422 | `ErrPermanent` | Unprocessable — domain not verified etc. |
| 429 | `ErrTransient` | Rate limited |
| 5xx | `ErrTransient` | Resend-side problem |

Idempotency note: an idempotent retry within 24h returns the *original*
message-id with the original status; subsequent invocations after the
window create a new message.
