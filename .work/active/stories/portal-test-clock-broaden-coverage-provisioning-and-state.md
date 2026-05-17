---
id: portal-test-clock-broaden-coverage-provisioning-and-state
kind: story
stage: implementing
tags: [testing, testability, portal]
parent: portal-test-clock-broaden-coverage
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Inject Clock into accounts, auth/provision, and oauth state-TTL paths

## Scope

Thread an injectable `Clock` through three closely-related provisioning
+ state-TTL surfaces:

1. `internal/portal/accounts.Handler` — org creation timestamp (`CreateOrg`),
   org-invite creation (`CreateOrgInvite`), and org-invite expiry check
   (`AcceptOrgInvite`).
2. `internal/portal/auth.FindOrProvision` (and its helper
   `createAccountAndOrg`) — account+org provisioning timestamp. Callers
   are `auth.MagicLinkHandler.ExchangeMagicLink` (already clock-aware
   via the v1 work) and `auth.OAuthHandler.OauthCallback` (deferred
   site — see Design decisions on the parent feature).
3. `internal/portal/oauth.StoreState` — OAuth-state row creation
   timestamp + expiry. Caller is `auth.OAuthHandler.StartOAuth`
   (deferred site; see below).

The auth/oauth.go and oauth/github.go files are locked by the in-flight
`portal-oauth-provider-error-taxonomy` feature; we touch the helpers
they call but NOT the handlers themselves. Wiring the
`OAuthHandler.NewWithClock` constructor is deferred to a follow-on
story once the oauth-taxonomy feature lands. The helpers (`StoreState`,
`FindOrProvision`) are still refactored so a future wiring is
mechanical.

## Files

### Modified

- `internal/portal/accounts/handlers.go` — add `Clock` interface,
  `realClock`, `clock` field on `Handler`, and `NewWithClock`
  constructor. Replace `time.Now().UTC()` in `CreateOrg` with
  `h.clock.Now()`.
- `internal/portal/accounts/orgs.go` — replace the 2 `time.Now().UTC()`
  reads in `CreateOrgInvite` and `AcceptOrgInvite` with `h.clock.Now()`.
- `internal/portal/auth/provision.go` — change
  `FindOrProvision(ctx, s, id)` to `FindOrProvision(ctx, s, id, now time.Time)`,
  and `createAccountAndOrg` similarly. Both callers
  (`magic_link.go`, `oauth.go`) pass their clock's `Now()`. Drop the
  internal `time.Now().UTC()` read in `createAccountAndOrg`.
- `internal/portal/auth/magic_link.go` — pass `h.clock.Now()` to
  `FindOrProvision`.
- `internal/portal/oauth/state.go` — change `StoreState(ctx, s,
  nonce, provider, redirectURI)` to `StoreState(ctx, s, nonce,
  provider, redirectURI, now time.Time)`. Drop the internal
  `time.Now().UTC()` read.
- `cmd/portal/main.go` — replace `accounts.New(...)` with the
  conditional `accounts.NewWithClock(...)` pattern (mirror the
  magic-link wiring). Add `accountsClock()` getter on
  `testClockProvider`.
- `cmd/portal/test_clock_advance.go` (modified, `//go:build e2etest`)
  — add `accountsClock()` returning `accounts.Clock` (returns
  `p.clock`).
- `cmd/portal/test_clock_advance_prod.go` (modified, `//go:build !e2etest`)
  — add `accountsClock()` returning `nil`.

### NOT modified (in-flight / deferred)

- `internal/portal/auth/oauth.go` — locked by
  `portal-oauth-provider-error-taxonomy`. The `time.Now().UTC()` read
  on line 110 (oauth-state expiry check) stays on the real clock until
  the oauth-taxonomy feature lands; a follow-on story will then add
  `NewOAuthHandlerWithClock` and pass `h.clock.Now()` to `StoreState`.
- The new `StoreState(..., now)` signature breaks the current
  call site in `oauth.go`. To preserve compile, this story adds a
  thin `StoreStateNow(ctx, s, ...)` wrapper in `oauth/state.go` that
  reads `time.Now().UTC()` and calls `StoreState`, and leaves the
  current `oauth.go` call site calling `StoreStateNow`. The wrapper
  is removed when the follow-on story lands. **Alternatively**, the
  cleaner option: keep `StoreState`'s signature unchanged and add a
  new `StoreStateAt(ctx, s, ..., now)` function. Implementor picks
  whichever yields the smaller diff; the goal is "don't touch
  oauth.go".

## Spec

### `internal/portal/accounts/handlers.go`

After the package doc block, add:

```go
// Clock is an injectable time source. The default realClock calls
// time.Now().UTC(); tests inject a fakeClock to simulate org-invite
// expiry. Shape mirrors internal/portal/auth.Clock and
// internal/portal/tokens.Clock so a single AdvanceableClock instance
// satisfies all three.
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }
```

Add `clock Clock` to the `Handler` struct.

Replace the existing `New` with:

```go
// New returns a Handler with the real system clock.
func New(s store.Store, sender senders.Sender, portalURL string) *Handler {
    return NewWithClock(s, sender, portalURL, realClock{})
}

// NewWithClock returns a Handler with the supplied clock.
func NewWithClock(s store.Store, sender senders.Sender, portalURL string, clock Clock) *Handler {
    return &Handler{store: s, sender: sender, portalURL: portalURL, clock: clock}
}
```

Replace `now := time.Now().UTC()` in `CreateOrg` (line 85) with
`now := h.clock.Now()`.

### `internal/portal/accounts/orgs.go`

Replace both `now := time.Now().UTC()` reads (lines 63 and 138) with
`now := h.clock.Now()`.

### `internal/portal/auth/provision.go`

Change the signature:

```go
func FindOrProvision(ctx context.Context, s store.Store, id Identity, now time.Time) (store.Account, store.Org, error) {
    ...
    return createAccountAndOrg(ctx, s, id, now)
}

func createAccountAndOrg(ctx context.Context, s store.Store, id Identity, now time.Time) (store.Account, store.Org, error) {
    // Drop the local `now := time.Now().UTC()`. Use the passed `now`.
    accountID := uuid.New().String()
    ...
}
```

### `internal/portal/auth/magic_link.go`

In `ExchangeMagicLink`, change:

```go
acc, _, err := FindOrProvision(ctx, h.store, id)
```

to:

```go
acc, _, err := FindOrProvision(ctx, h.store, id, h.clock.Now())
```

The local `now := h.clock.Now()` is already in scope, so:

```go
acc, _, err := FindOrProvision(ctx, h.store, id, now)
```

is preferred (reuses the same instant for consume + provision).

### `internal/portal/oauth/state.go`

Pick ONE of these two refactor shapes (implementor's call — diff
size is the tie-breaker):

**Option A (preferred):** Add a new function, keep the old one:

```go
// StoreStateAt inserts a fresh state nonce using the supplied
// timestamp as created_at. ExpiresAt is now + StateNonceTTL. Used by
// clock-injectable callers.
func StoreStateAt(ctx context.Context, s store.OAuthStateStore, nonce, provider, redirectURI string, now time.Time) error {
    return s.InsertOAuthState(ctx, store.InsertOAuthStateParams{
        Nonce:       nonce,
        Provider:    provider,
        RedirectURI: redirectURI,
        CreatedAt:   now,
        ExpiresAt:   now.Add(StateNonceTTL),
    })
}
```

Keep `StoreState` unchanged so `auth/oauth.go` keeps compiling without
modification.

**Option B:** Change `StoreState`'s signature; rewrite the call site
in `auth/oauth.go`. **DO NOT use this option** — `auth/oauth.go` is
in-flight.

### `internal/portal/auth/oauth.go`

NOT MODIFIED. Stays on `time.Now().UTC()` for the state-TTL check on
line 110. The state row's `CreatedAt`/`ExpiresAt` timestamps come from
`StoreState` (still real clock until a follow-on story rewires it).

### `cmd/portal/test_clock_advance.go` / `test_clock_advance_prod.go`

Add `accountsClock()` method mirroring `magicLinkClock()`:

```go
// e2etest variant:
func (p *testClockProvider) accountsClock() accounts.Clock { return p.clock }

// production variant:
func (p *testClockProvider) accountsClock() accounts.Clock { return nil }
```

### `cmd/portal/main.go`

Replace `accountsHandler := accounts.New(dbStore, emailSender, cfg.PortalURL)`
with:

```go
var accountsHandler *accounts.Handler
if c := testClk.accountsClock(); c != nil {
    accountsHandler = accounts.NewWithClock(dbStore, emailSender, cfg.PortalURL, c)
} else {
    accountsHandler = accounts.New(dbStore, emailSender, cfg.PortalURL)
}
```

## Acceptance criteria

- [ ] `accounts.NewWithClock` exists and is wired in `main.go` via the
      same e2etest-gated pattern as the magic-link handler.
- [ ] All 3 `time.Now().UTC()` reads in `accounts/handlers.go` and
      `accounts/orgs.go` go through the injected clock.
- [ ] `auth.FindOrProvision` accepts a `now time.Time` parameter and
      no longer reads `time.Now()` internally. Its only call site in
      this story (`magic_link.go`) passes `h.clock.Now()`.
- [ ] `oauth.StoreStateAt` (or equivalent) accepts a `now time.Time`
      parameter and is fully functional, even if not yet called from
      the (locked) `auth/oauth.go` handler.
- [ ] `auth/oauth.go` is NOT modified by this story (in-flight conflict).
- [ ] `go build ./...` and `go build -tags e2etest ./...` both succeed.
- [ ] Existing `magic_link_ttl_expiry` chaos subtest still passes
      end-to-end (regression check on the magic-link path).
- [ ] Unit tests for `accounts/handlers.go` and `accounts/orgs.go` pass.

## Test approach

- Add a `fakeClock` to `accounts` package's existing test file (or
  the new `accounts/clock_test.go` if no existing file) that lets a
  unit test exercise the org-invite expiry branch deterministically.
- Add a unit test asserting `AcceptOrgInvite` returns 401
  `auth.invalid_token` when the clock is past `invite.ExpiresAt`.
- Add a unit test for `oauth.StoreStateAt` asserting the row's
  `ExpiresAt` equals `now + StateNonceTTL`.
- No new e2e test required — the v1 magic-link TTL test exercises the
  `FindOrProvision` clock path implicitly.

## Production-safety verification

- `git grep -- 'testclock' cmd/portal/ internal/portal/` returns only
  build-tag-gated files plus the prod stub.
- `go build ./cmd/portal/` (no tags) produces a binary with no
  `testclock` symbols (verify via `go tool nm`).
- All new exported clock interfaces (`accounts.Clock`) have zero-arg
  `Now() time.Time` shape that's structurally compatible with the
  existing `auth.Clock`, `tokens.Clock`, and
  `*testclock.AdvanceableClock`.
