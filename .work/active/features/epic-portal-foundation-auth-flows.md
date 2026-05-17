---
id: epic-portal-foundation-auth-flows
kind: feature
stage: implementing
tags: [portal, security]
parent: epic-portal-foundation
depends_on: [epic-portal-foundation-tokens]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Foundation — Auth Flows

## Brief

The two user-facing authentication flows that produce a session token at
the end: **delegated OAuth** (GitHub for v1, with a provider interface so
Google/OIDC can land later without rework) and **magic-link** (always
available, no password concept ever — the canonical headless self-host
flow and the standard alternative on the locked sign-in card).

Both flows terminate in a call to the tokens feature's issuance helper.
Both flows also trigger **org auto-provisioning on first sign-in**: a new
account triggers creation of a personal org and a `members` row binding
account-to-org with role `creator` (the operator semantics on a fresh
self-host).

**Endpoints delivered** (per `docs/PROTOCOL.md > REST API > Auth`):

- `POST /api/auth/oauth/start` — returns the provider authorization URL
  (with state nonce stored server-side)
- `POST /api/auth/oauth/callback` — exchanges authorization code for
  provider access, resolves provider identity → portal account (creating
  if first sign-in), issues portal token
- `POST /api/auth/magic-link/request` — accepts email, generates a
  `magic_link_tokens` row (single-use, 15-minute TTL), sends the link via
  the configured email provider
- `POST /api/auth/magic-link/exchange` — accepts the magic-link token,
  marks it `used_at`, resolves/creates account, issues portal token

**Email provider abstraction**: a `Sender` interface with `Send(ctx,
recipient, subject, body) error`, with concrete implementations for SMTP
(self-host default), SendGrid, Postmark, and Resend. Selection via config
(`email.provider: smtp|sendgrid|postmark|resend` + per-provider settings).
Adding a provider is a new file implementing `Sender` and a config switch
entry — no other code touched.

**OAuth provider abstraction**: a `Provider` interface that exposes
authorization URL builder, code-to-identity exchange, and an identity
struct (provider id + email + display name). GitHub implementation
ships. Adding a provider (Google, generic OIDC) is a new file
implementing `Provider` + a config switch.

Does NOT cover the visual sign-in surface (`.mockups/flows/onboarding/02-sign-in.html`
is the locked target; epic-portal-ui-foundation implements the
client-side UI that posts to these endpoints). Does NOT cover account
management endpoints (those belong to the accounts feature).

## Epic context

- Parent epic: `epic-portal-foundation`
- Position in epic: consumes the tokens feature for issuance; consumed
  by the accounts feature (logged-in user needed for management surface).

## Foundation references

- `docs/PROTOCOL.md` — REST API > Auth section
- `docs/SECURITY.md` — Authentication > User authentication
  (OAuth + magic-link flow descriptions), Token lifetime and renewal
- `docs/UX.md` — Flow: joining a session (where these flows fire)
- `.mockups/flows/onboarding/02-sign-in.html` — locked sign-in card
  (OAuth + magic-link equally prominent)

## Inherited epic design decisions

- **OAuth providers v1**: GitHub only, behind a `Provider` interface.
  Google/OIDC add cleanly later.
- **Magic-link anti-replay**: single-use, 15-minute TTL, stored in
  `magic_link_tokens` with `used_at` timestamp. Second click returns
  "this link was already used" via the standard error contract.
- **Org auto-provisioning**: first sign-in via either flow creates a
  personal org + `members` row with role `creator`. No special
  bootstrap dance for self-host operators.
- **Email provider abstraction**: pluggable `Sender` interface; SMTP
  default for self-host, SendGrid/Postmark/Resend available via config.

## Decomposition risks

- The email provider abstraction is the most novel piece. If SMTP and
  the 3 hosted providers' error / retry semantics diverge significantly,
  the design pass may need to spike a 4-provider table of edge cases
  before locking the interface.
- Magic-link delivery latency depends on the email provider's queueing
  behavior. The 15-minute TTL is generous but design-time we should
  document how long-tail delivery interacts with the TTL.

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Email provider versions**: defer to the auto-loaded `email-senders`
  skill for verified library pins. Likely set: `wneessen/go-mail` for
  SMTP, `sendgrid/sendgrid-go`, `mrz1836/postmark`, `resend/resend-go`.
- **Sender interface**: synchronous `Send(ctx, recipient, subject, body)
  error` per the locked epic decision. Per-provider implementations
  translate the call; errors are wrapped in a normalized
  `sender.ErrTransient` vs `sender.ErrPermanent` taxonomy so callers
  can distinguish retry-able failures.
- **Sender factory**: `senders.New(cfg) (Sender, error)` dispatches on
  `cfg.Provider` string. Config schema added to `config.Config`
  via a new `Email EmailConfig` field (struct with `Provider` +
  per-provider sub-structs).
- **OAuth provider interface**:
  ```go
  type Provider interface {
      Name() string                                                // "github" | "google"
      AuthorizeURL(state, redirectURI string) string               // builds the URL
      Exchange(ctx, code, redirectURI string) (Identity, error)    // code -> identity
  }
  type Identity struct {
      ProviderID  string  // GitHub user id (string form)
      Email       string
      DisplayName string
  }
  ```
- **OAuth state storage**: a transient table `oauth_state` with
  `nonce`, `created_at`, `expires_at` (5-minute TTL). Schema added
  by THIS feature as `00003_oauth_state.sql` migration. Verified
  + consumed on callback.
- **Auto-provisioning helper**: shared by both OAuth callback and
  magic-link exchange. Function signature:
  ```go
  func provisionOnFirstSignIn(ctx, store, identity Identity) (account, org, err)
  ```
  Looks up account by `(provider, provider_id)` for OAuth or by
  email for magic-link. If not found: creates account, creates org
  (slug = email-prefix-or-username, deduped if collision), inserts
  org_member with role `creator`. Returns existing or created
  account + their primary org.
- **Magic-link table**: already in schema (`magic_link_tokens`)
  from `data-layer`. Email is the link's target — the
  exchange step uses the email to find-or-create the account.
- **Magic link URL**: `<portal_url>/auth/magic-link?token=<raw>`.
  The link points at the SPA, which then POSTs the token to
  `/api/auth/magic-link/exchange`. The SPA route is registered by
  `epic-portal-ui-foundation` (already at done).
- **GitHub OAuth specifics**: client_id, client_secret in config
  (`EmailConfig.GitHub.ClientID`, `.ClientSecret`). Redirect URI
  is `<portal_url>/auth/oauth/callback` (handled by the SPA).
- **Story decomposition**: 2 stories.
  1. `sender-and-magic-link` — Sender interface + 4 senders +
     magic-link endpoints + auto-provisioning helper +
     openapi additions. depends_on: []
  2. `oauth-provider-github` — Provider interface + GitHub
     implementation + OAuth endpoints. depends_on:
     [sender-and-magic-link] (shares auto-provisioning helper +
     openapi spec)

## Architectural choice

**Two abstractions, four concrete implementations on the Sender
side, one concrete implementation on the Provider side. Both
flows funnel through a shared `provisionOnFirstSignIn` helper
that creates account + org + membership atomically. Both terminate
in a `tokens.Service.Issue` call.**

## Implementation Units

### Unit 1: Sender interface + factory

**File**: `internal/portal/senders/sender.go`
**Story**: `epic-portal-foundation-auth-flows-sender-and-magic-link`

```go
package senders

import "context"

type Sender interface {
    Send(ctx context.Context, recipient, subject, body string) error
}

var (
    ErrTransient = errors.New("senders: transient error")
    ErrPermanent = errors.New("senders: permanent error")
)
```

### Unit 2: SMTP, SendGrid, Postmark, Resend implementations

**Files**:
- `internal/portal/senders/smtp.go` — using `wneessen/go-mail` per the email-senders skill
- `internal/portal/senders/sendgrid.go`
- `internal/portal/senders/postmark.go`
- `internal/portal/senders/resend.go`
- `internal/portal/senders/factory.go` — `New(cfg config.EmailConfig) (Sender, error)`

Each provider's error mapping translates 4xx → `ErrPermanent`,
5xx + network errors → `ErrTransient`.

### Unit 3: Config additions

**File**: `internal/portal/config/config.go` (edit)

```go
type EmailConfig struct {
    Provider string `yaml:"provider"` // smtp|sendgrid|postmark|resend
    From     string `yaml:"from"`     // sender address
    SMTP     struct {
        Host string `yaml:"host"`
        Port int    `yaml:"port"`
        User string `yaml:"user"`
        Pass string `yaml:"pass"`
        TLS  bool   `yaml:"tls"`
    } `yaml:"smtp"`
    SendGrid struct {
        APIKey string `yaml:"api_key"`
    } `yaml:"sendgrid"`
    Postmark struct {
        ServerToken string `yaml:"server_token"`
    } `yaml:"postmark"`
    Resend struct {
        APIKey string `yaml:"api_key"`
    } `yaml:"resend"`
}
```

Plus env overrides for all keys.

### Unit 4: Auto-provisioning helper

**File**: `internal/portal/auth/provision.go`

```go
package auth

import (
    "context"

    "jamsesh/internal/db/store"
)

type Identity struct {
    Provider    string  // "github" | "magic-link"
    ProviderID  string  // GitHub user ID; empty for magic-link
    Email       string
    DisplayName string
}

// FindOrProvision returns the account+org pair for the given identity,
// creating them on first sign-in. Idempotent.
func FindOrProvision(ctx context.Context, s store.Store, id Identity) (store.Account, store.Org, error)
```

Algorithm:
1. Lookup by email (or by GitHub user id for OAuth)
2. If found: return account + primary org (first org member row)
3. If not: create account (ULID), create org (slug derived from email prefix + suffix-if-collision), insert org_member with role `creator`

### Unit 5: Magic-link endpoints

**Files**:
- `internal/portal/auth/magic_link.go` — handler logic
- `docs/openapi.yaml` (edit) — add `/api/auth/magic-link/request` and `/api/auth/magic-link/exchange`

`request`: accepts `{email}`. Generates 32-byte random token, hashes (SHA-256), inserts into `magic_link_tokens` with 15-min expiry, sends email via `Sender.Send`.

`exchange`: accepts `{token}`. Hashes, looks up in `magic_link_tokens`, validates not used/expired, marks `used_at`, calls `FindOrProvision`, calls `tokens.Service.Issue`, returns `TokenPair`.

### Unit 6: OAuth Provider interface + GitHub

**Files**:
- `internal/portal/oauth/provider.go` — Provider interface, Identity struct
- `internal/portal/oauth/github.go` — GitHub implementation
- `internal/portal/oauth/state.go` — state nonce storage (uses the new `oauth_state` table)
- `internal/db/migrations/{sqlite,postgres}/00003_oauth_state.sql` — schema addition
- Store interface extension for `oauth_state` queries
**Story**: `epic-portal-foundation-auth-flows-oauth-provider-github`

GitHub specifics:
- AuthorizeURL: `https://github.com/login/oauth/authorize?client_id=...&redirect_uri=...&state=...&scope=read:user user:email`
- Exchange: POST to `https://github.com/login/oauth/access_token` with code → GitHub access token → GET `/user` and `/user/emails` to extract identity

### Unit 7: OAuth endpoints

**Files**:
- `internal/portal/auth/oauth.go` — handler logic
- `docs/openapi.yaml` (edit) — add `/api/auth/oauth/start` and `/api/auth/oauth/callback`

`start`: accepts `{provider}` (just "github" for v1). Generates state nonce, inserts in `oauth_state` with 5-min TTL, builds AuthorizeURL, returns `{authorize_url}`.

`callback`: accepts `{provider, code, state}`. Validates state nonce (delete from `oauth_state` on use), calls `provider.Exchange`, calls `FindOrProvision`, calls `tokens.Service.Issue`, returns `TokenPair`.

## Story decomposition

1. `epic-portal-foundation-auth-flows-sender-and-magic-link` — Units 1-5
2. `epic-portal-foundation-auth-flows-oauth-provider-github` — Units 6-7

## Implementation Order

1. sender-and-magic-link
2. oauth-provider-github

## go.mod additions

- `github.com/wneessen/go-mail` — SMTP
- `github.com/sendgrid/sendgrid-go` — SendGrid
- `github.com/mrz1836/postmark` — Postmark
- `github.com/resend/resend-go/v2` — Resend
- (verify exact pins via `email-senders` skill)

## Testing

- Each Sender's smoke test against a mock server (httptest for hosted; bufio/net for SMTP)
- `FindOrProvision` round-trip: new identity → creates; existing → returns
- Magic-link single-use enforcement: second exchange of same token → error
- OAuth state-nonce mismatch → 400
- OAuth state-nonce expired → 400
- End-to-end magic-link flow: request → email captured → exchange → token returned

## Risks

- **Email provider config explosion**: 4 providers with different
  required fields. Mitigation: only the chosen provider's struct
  needs valid values; others can be empty. Validation runs on
  the chosen provider.
- **GitHub OAuth client registration**: requires manually
  creating a GitHub OAuth app per-deployment. Operator concern,
  documented in `docs/SELF_HOST.md` already.
- **Org slug collision**: two users with same email prefix
  (`bob@a.com` and `bob@b.com`). Collision-resolved by appending
  a number suffix; documented in implementation notes.
- **`oauth_state` cleanup**: expired nonces accumulate. Mitigation:
  cleanup on insert (delete WHERE expires_at < now) — cheap on
  small tables. Or operator-cron — same as oauth_tokens.
