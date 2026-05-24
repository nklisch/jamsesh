---
id: story-refactor-per-package-clock-compliance-auth
kind: story
stage: done
tags: [portal, refactor, testing]
parent: feature-refactor-per-package-clock-compliance
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# auth package: route OAuthHandler + FindOrProvision through the existing Clock

## Brief

`internal/portal/auth/` already defines a `Clock` interface and `realClock`
in `magic_link.go` (used by `MagicLinkHandler`). Two sibling code paths in
the same package bypass it and call `time.Now()` directly:

- `auth/oauth.go:110` — `if time.Now().UTC().After(stateRow.ExpiresAt) {` in
  the OAuth callback's state-expiry check. `OAuthHandler` has no clock field.
- `auth/provision.go:43` — `return FindOrProvisionAt(ctx, s, id, time.Now().UTC())`.
  The clock-injectable variant already exists (`FindOrProvisionAt`); this
  wrapper is the only call site that defeats it.

This story brings both into compliance with `per-package-clock-interface`.

## Current state

```go
// oauth.go (around line 19, 110)
type OAuthHandler struct {
    providers map[string]portaloauth.Provider
    store     store.Store
    portalURL string
}

func NewOAuthHandler(providers map[string]portaloauth.Provider, store store.Store, portalURL string) *OAuthHandler {
    return &OAuthHandler{providers: providers, store: store, portalURL: portalURL}
}

func (h *OAuthHandler) OauthCallback(...) {
    // ...
    if time.Now().UTC().After(stateRow.ExpiresAt) {
        return oauthBadRequest("oauth.expired_state", ...), nil
    }
    // ...
}

// provision.go (around line 32)
func FindOrProvision(ctx context.Context, s store.TxStore, id auth.Identity) (...) {
    return FindOrProvisionAt(ctx, s, id, time.Now().UTC())
}
```

## Target state

```go
// oauth.go
type OAuthHandler struct {
    providers map[string]portaloauth.Provider
    store     store.Store
    portalURL string
    clock     Clock
}

func NewOAuthHandler(providers ..., store ..., portalURL string) *OAuthHandler {
    return NewOAuthHandlerWithClock(providers, store, portalURL, realClock{})
}

func NewOAuthHandlerWithClock(providers ..., store ..., portalURL string, clock Clock) *OAuthHandler {
    return &OAuthHandler{providers: providers, store: store, portalURL: portalURL, clock: clock}
}

func (h *OAuthHandler) OauthCallback(...) {
    // ...
    if h.clock.Now().After(stateRow.ExpiresAt) {
        return oauthBadRequest("oauth.expired_state", ...), nil
    }
    // ...
}

// provision.go — convert callers of FindOrProvision to use FindOrProvisionAt
// with their own clock. If no callers remain, delete FindOrProvision.
```

## Implementation notes

- The package's existing `Clock` interface and `realClock` (in `magic_link.go`)
  satisfy this need — do NOT redefine. `OAuthHandler` gets the same `Clock`
  field as `MagicLinkHandler`.
- `OauthCallback` line 110: `time.Now().UTC().After(...)` becomes
  `h.clock.Now().After(...)`. Note `realClock.Now()` already returns UTC, so
  the wording shortens.
- `FindOrProvision`: identify every caller via grep (`grep -rn "auth.FindOrProvision\b\|FindOrProvision(" internal/`).
  For each caller, either thread their existing clock into a direct
  `FindOrProvisionAt` call, or — if the caller has no clock and gains
  nothing from injection — leave it on `FindOrProvision`. Goal: every
  call site that has a clock should route around the wrapper. The
  wrapper itself can stay (still useful for boot-path code with no
  clock) but the production call sites of clock-aware handlers must use
  `FindOrProvisionAt`.
- Add a unit test for `OAuthHandler` that uses a fake clock to drive the
  `state expired` branch deterministically — currently the only way to hit
  that branch in tests is to wait or manipulate `stateRow.ExpiresAt`.

## Acceptance criteria

- [ ] `OAuthHandler` carries a `clock Clock` field; constructor pair
      (`NewOAuthHandler` + `NewOAuthHandlerWithClock`) mirrors `MagicLinkHandler`.
- [ ] `oauth.go:110` reads `h.clock.Now()` instead of `time.Now().UTC()`.
- [ ] At least one OAuth caller (the one that drove this discovery — likely the
      magic-link → OAuth integration test) uses `FindOrProvisionAt` directly
      with the caller's clock.
- [ ] A new test in `internal/portal/auth/oauth_test.go` (or extension of an
      existing test) drives the expired-state branch via a fake clock.
- [ ] `go build ./...` clean.
- [ ] `go test ./internal/portal/auth/...` clean.

## Risk

**Low.** The Clock interface and pattern already exist in the same package; no
cross-package coupling introduced. The constructor pair pattern keeps existing
callers compiling without changes.

## Rollback

`git revert` the implementation commit. No schema/state changes; the
`time.Now().UTC()` form returns the same value as `realClock{}.Now()` in
production.

## Out of scope

- `auth/slug.go:66` (`time.Now().UnixNano()` as PRNG seed) — tracked
  separately under a non-`[refactor]` story; replacing the seed changes
  RNG behavior.

## Implementation notes

### Discovery

`grep -rn "auth.FindOrProvision\b\|\.FindOrProvision(" internal/ cmd/` found
callers only in `provision_test.go` (test-only; uses `FindOrProvision` as a
convenience, no clock needed) and `oauth.go` (the single production caller that
had a clock). No boot-path or other non-handler callers. `provision_test.go`
callers were left as-is — they are tests that don't care about clock-stamped
timestamps, and `FindOrProvision` wrapper remains valid for that use.

`NewOAuthHandler` is called from `cmd/portal/main.go:431` and four test sites
in `oauth_test.go`; all use the positional constructor and required zero
signature changes.

### Changes made

**`internal/portal/auth/oauth.go`**
- Added `clock Clock` field to `OAuthHandler`.
- Replaced `NewOAuthHandler` body with a delegation to new
  `NewOAuthHandlerWithClock(..., realClock{})` — existing callers unaffected.
- Added `NewOAuthHandlerWithClock` for test injection.
- `OauthCallback` line 110: `time.Now().UTC().After(stateRow.ExpiresAt)` →
  `h.clock.Now().After(stateRow.ExpiresAt)`.
- `OauthCallback` `FindOrProvision` call → `FindOrProvisionAt(ctx, h.store, id, h.clock.Now())`.
- Removed now-unused `"time"` import.

**`internal/portal/auth/oauth_test.go`**
- Replaced the weak `TestOAuthCallback_ExpiredState_Returns400` (which just
  tested a missing nonce) with a deterministic fake-clock test: injects
  `&fakeClock{}` via `NewOAuthHandlerWithClock`, stores a valid nonce, advances
  the clock 6 minutes past the 5-minute TTL, calls callback — asserts 400 with
  `oauth.expired_state` error code.

### `FindOrProvision` caller list

- `internal/portal/auth/provision_test.go` — test only, no clock, left as-is.
- `internal/portal/auth/oauth.go` — updated to `FindOrProvisionAt(..., h.clock.Now())`.

### Verification

`go build ./...` — clean.
`go test ./internal/portal/auth/...` — all pass (19 tests including new expired-state test).
`go test ./...` — auth and all other packages pass; three pre-existing build
failures in `githttp`, `postreceive`, `objectstore` belong to another
in-progress story and were present before this change.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Pattern-compliant clock injection — matches the canonical `events.Log` / `auth.MagicLinkHandler` shape. Production wiring unchanged (`realClock{}` returns same value as `time.Now()`). Tests demonstrate deterministic time control with no wall-clock waits. `go build ./...` and `go test ./...` clean.
