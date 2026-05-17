# Email config shape (jamsesh portal)

Single `email.provider` key selects the adapter. Provider-specific
credentials live under `email.<provider>.*`. Sample YAML:

```yaml
email:
  provider: smtp        # smtp | sendgrid | postmark | resend
  from: "jamsesh <noreply@example.com>"
  reply_to: ""          # optional, sent as Reply-To
  tag: "magic-link"     # forwarded to providers that support tagging

  smtp:
    host: "smtp.example.com"
    port: 587
    username: "apikey"
    password: ""        # via env var EMAIL_SMTP_PASSWORD
    tls: "mandatory"    # mandatory | opportunistic | none

  sendgrid:
    api_key: ""         # via env var EMAIL_SENDGRID_API_KEY

  postmark:
    server_token: ""    # via env var EMAIL_POSTMARK_SERVER_TOKEN
    account_token: ""   # optional, account-level admin operations
    message_stream: "outbound"

  resend:
    api_key: ""         # via env var EMAIL_RESEND_API_KEY
```

## Env-var overrides

Standard portal pattern: any leaf string can be overridden by the
matching `EMAIL_<UPPER_SNAKE_CASE>` env var. Sensitive fields (api keys,
SMTP password) SHOULD be passed via env or external secrets only — the
on-disk YAML should hold the empty string in those positions for
self-host configs.

## Validation

Config loader rejects:

- `provider: smtp` with empty `host` or `port`.
- `provider: <hosted>` with empty `api_key` / `server_token`.
- `from` empty regardless of provider.
- `tls: none` AND `port: 587|465` (suggests misconfig — fail loudly).

## Selection at startup

```go
func NewFromConfig(cfg Config) (Sender, error) {
    switch cfg.Provider {
    case "smtp":
        return NewSMTPSender(cfg.SMTP), nil
    case "sendgrid":
        return NewSendGridSender(cfg.SendGrid.APIKey), nil
    case "postmark":
        return NewPostmarkSender(cfg.Postmark.ServerToken, cfg.Postmark.AccountToken), nil
    case "resend":
        return NewResendSender(cfg.Resend.APIKey), nil
    default:
        return nil, fmt.Errorf("email: unknown provider %q", cfg.Provider)
    }
}
```

One construction site; everything downstream depends only on `Sender`.
