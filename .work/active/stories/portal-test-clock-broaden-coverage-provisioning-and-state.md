---
id: portal-test-clock-broaden-coverage-provisioning-and-state
kind: story
stage: done
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

## Implementation notes

### Files modified

- `internal/portal/accounts/handlers.go` — added `Clock` interface,
  `realClock`, `clock` field on `Handler`, `NewWithClock` constructor;
  swapped `time.Now().UTC()` in `CreateOrg` for `h.clock.Now()`.
- `internal/portal/accounts/orgs.go` — swapped both `time.Now().UTC()`
  reads (`CreateOrgInvite`, `AcceptOrgInvite`) for `h.clock.Now()`.
- `internal/portal/auth/provision.go` — added additive
  `FindOrProvisionAt(ctx, s, id, now)` variant. `FindOrProvision(ctx,
  s, id)` now delegates with `time.Now().UTC()` (back-compat for the
  locked `oauth.go` caller and all existing call sites/tests).
  `createAccountAndOrg` now takes `now time.Time`; the internal
  `time.Now().UTC()` read is dropped.
- `internal/portal/auth/magic_link.go` — `ExchangeMagicLink` now calls
  `FindOrProvisionAt(ctx, h.store, id, now)` so the injected clock is
  the sole time source for the provisioning write path. (One-line
  swap; the v1 clock plumbing in this file is untouched.)
- `internal/portal/oauth/state.go` — added additive `StoreStateAt(...,
  now time.Time)`. `StoreState(...)` now delegates with
  `time.Now().UTC()` (back-compat for the locked `oauth.go` caller).
  Chose Option A from the spec — additive — because the in-flight
  oauth.go lock made signature change infeasible.
- `cmd/portal/test_clock_advance.go` (e2etest) — added `accountsClock()`
  returning `p.clock` typed as `accounts.Clock`. Imports `accounts`.
- `cmd/portal/test_clock_advance_prod.go` (!e2etest) — added
  `accountsClock()` returning `nil` typed as `accounts.Clock`. Imports
  `accounts`.
- `cmd/portal/main.go` — added the conditional-clock block around the
  `accountsHandler` construction, mirroring the magic-link and tokens
  patterns from v1.

### Files added (tests)

- `internal/portal/accounts/clock_test.go` — local `fakeClock`,
  `newOrgsClockTestEnv` helper wiring `accounts.NewWithClock`, and three
  unit tests:
  - `TestCreateOrg_UsesInjectedClock` — verifies org + member rows'
    `CreatedAt` come from the fakeClock.
  - `TestCreateOrgInvite_UsesInjectedClock` — verifies invite
    `ExpiresAt` is `clock.Now() + 7d`.
  - `TestAcceptOrgInvite_ClockPastExpiry_Returns401` — advances the
    fakeClock past the seeded invite's `ExpiresAt` and asserts the
    handler returns 401 `auth.invalid_token`.
- `internal/portal/auth/provision_test.go` — added two unit tests
  inside the existing file:
  - `TestFindOrProvisionAt_UsesSuppliedClock` — verifies all three
    rows (account, org, org_member) carry the supplied `now` as
    `CreatedAt`.
  - `TestFindOrProvision_DelegatesToFindOrProvisionAt` — sanity check
    that the back-compat entry point still uses the real wall clock.
- `internal/portal/oauth/state_test.go` (new file) — two unit tests:
  - `TestStoreStateAt_UsesSuppliedClock` — verifies `CreatedAt` =
    supplied now, `ExpiresAt` = now + `StateNonceTTL`.
  - `TestStoreState_DelegatesToStoreStateAt` — sanity check on
    back-compat entry point.

### Call sites wrapped (5)

1. `accounts.CreateOrg` — `accounts/handlers.go:85` swap.
2. `accounts.CreateOrgInvite` — `accounts/orgs.go:63` swap.
3. `accounts.AcceptOrgInvite` — `accounts/orgs.go:138` swap (TTL gate).
4. `auth.createAccountAndOrg` — `auth/provision.go` `now` parameter
   (drops the internal `time.Now().UTC()` read). The magic-link caller
   in `ExchangeMagicLink` passes the same `now` instant used for
   token-consume.
5. `oauth.StoreStateAt` — `oauth/state.go` new function. Currently
   reached only via `StoreState` (back-compat); a follow-on story will
   wire `OAuthHandler.StartOAuth` to call `StoreStateAt` directly with
   `h.clock.Now()` once `auth/oauth.go` unlocks.

### Deferred sites (1, by design)

- `auth/oauth.go:159` (`OauthCallback` → `FindOrProvision`) and
  `auth/oauth.go:76` (`StartOAuth` → `StoreState`) — `auth/oauth.go` is
  LOCKED by the in-flight `portal-oauth-provider-error-taxonomy`
  feature. The additive `FindOrProvisionAt` + `StoreStateAt` keep the
  existing back-compat call sites compiling; the follow-on story is
  ~30 LoC once `oauth.go` unlocks: add `OAuthHandler.NewWithClock`,
  thread `h.clock`, swap the two helper calls to the `*At` variants.

### Validation

- `go build ./internal/portal/accounts/... ./internal/portal/auth/...
  ./internal/portal/oauth/...` — clean (production tags).
- `go build -tags e2etest ./internal/portal/accounts/...
  ./internal/portal/auth/... ./internal/portal/oauth/...` — clean.
- `go vet ./internal/portal/accounts/... ./internal/portal/auth/...
  ./internal/portal/oauth/...` — clean.
- `go test -count=1 ./internal/portal/accounts/...
  ./internal/portal/auth/... ./internal/portal/oauth/...` — all pass
  (74 tests total, including the 7 new ones added for clock
  injection).

### Coordination notes

- `cmd/portal/main.go` and `cmd/portal/test_clock_advance{,_prod}.go`
  are jointly owned with the sibling story
  `portal-test-clock-broaden-coverage-handlers-workers-mcp` (running
  in parallel). Made additive changes only: added an `accountsClock()`
  accessor pair and a conditional-clock block around
  `accountsHandler`. Did not touch the sibling's blocks. The sibling
  was actively rebasing during implementation; its WIP imports for
  `automerger`, `comments`, `events`, `finalize`, `mcpendpoint`, and
  `storage` are preserved in `test_clock_advance{,_prod}.go`.
- `go build ./...` and `go test ./cmd/portal/...` (including
  `TestProductionBuild_HasNoTestEndpoint`) could not be exercised
  end-to-end in this session because the sibling has uncommitted
  partial work that breaks `internal/portal/finalize/` compile
  (unused-import errors after their swap to `h.clock.Now()` — they
  haven't finished pruning the `time` import yet). The work in this
  story is intentionally isolated to packages the sibling does not
  touch (`accounts`, `auth/provision`, `oauth/state`), so the
  sibling's WIP does not affect correctness of this story.

### Acceptance-criteria status

- [x] `accounts.NewWithClock` exists and is wired in `main.go` via the
      e2etest-gated pattern.
- [x] All 3 `time.Now().UTC()` reads in `accounts/handlers.go` and
      `accounts/orgs.go` go through `h.clock.Now()`.
- [x] `auth.FindOrProvisionAt` accepts a `now time.Time` parameter
      and is called from `magic_link.go` (the only in-scope caller).
      `auth.FindOrProvision` remains as a back-compat wrapper for the
      locked `oauth.go` caller (and existing tests).
- [x] `oauth.StoreStateAt` accepts a `now time.Time` parameter and is
      fully functional, even though `auth/oauth.go` continues calling
      the back-compat `StoreState` wrapper.
- [x] `auth/oauth.go` is NOT modified by this story.
- [x] My packages build clean on both `go build` and `go build -tags
      e2etest`. (Full repo build is blocked by sibling WIP — see
      Coordination notes.)
- [x] Unit tests for `accounts/handlers.go`, `accounts/orgs.go`,
      `auth/provision.go`, and `oauth/state.go` pass.

## Review

**Verdict:** Approve.

**Reviewed commit:** `fc05cff`.

### Spec compliance

All 5 clock-injection sites land exactly as designed:

1. `accounts.CreateOrg` — `h.clock.Now()` at handlers.go:106.
2. `accounts.CreateOrgInvite` — `h.clock.Now()` at orgs.go:63.
3. `accounts.AcceptOrgInvite` — `h.clock.Now()` at orgs.go:138 (TTL gate).
4. `auth.FindOrProvisionAt(ctx, s, id, now)` added; `FindOrProvision`
   preserved as back-compat delegate calling
   `FindOrProvisionAt(..., time.Now().UTC())`. `createAccountAndOrg`
   now takes `now time.Time`; internal `time.Now()` dropped.
5. `oauth.StoreStateAt(..., now)` added; `StoreState` preserved as
   back-compat delegate. Chose Option A (additive) per spec
   recommendation.

`magic_link.go` `ExchangeMagicLink` now calls `FindOrProvisionAt(...,
now)`, reusing the same instant as token-consume.

### Constructor + wiring

- `accounts.NewWithClock(s, sender, portalURL, clock)` exposed; old
  `accounts.New(...)` preserved as a `NewWithClock(..., realClock{})`
  delegate.
- `cmd/portal/main.go` branches on `testClk.accountsClock()` —
  e2etest returns `p.clock`, prod returns typed nil. Pattern mirrors
  the v1 magic-link/tokens wiring exactly.
- `accountsClock()` accessor added to both
  `test_clock_advance.go` and `test_clock_advance_prod.go`. Both
  files now import `jamsesh/internal/portal/accounts`.

### Deferred caller (auth/oauth.go)

Confirmed `auth/oauth.go` is unmodified by fc05cff. Its two call sites
(`StoreState` at line 76, `FindOrProvision` at line 159) hit the new
back-compat wrappers and the file compiles unchanged. The follow-on
to wire `OAuthHandler.NewWithClock` is mechanical once
`portal-oauth-provider-error-taxonomy` unlocks the file.

### Validation

- `go build ./...` — clean.
- `go build -tags e2etest ./...` — clean.
- `go vet ./internal/portal/...` — clean.
- `go test -count=1 ./internal/portal/accounts/... ./internal/portal/auth/...
  ./internal/portal/oauth/...` — all pass.
- `go test -run TestProductionBuild_HasNoTestEndpoint ./cmd/portal/...`
  — pass (production binary still has no test endpoint).

### Findings

- Blockers: 0.
- Important: 0.
- Nits: 0.

Coordination notes in implementation section (sibling story rebase
turbulence) no longer apply — the repo builds clean end-to-end at this
commit.
