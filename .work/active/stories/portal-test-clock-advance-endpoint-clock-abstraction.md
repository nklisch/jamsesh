---
id: portal-test-clock-advance-endpoint-clock-abstraction
kind: story
stage: implementing
tags: [testing, testability]
parent: portal-test-clock-advance-endpoint
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Magic-link handler: thread an injectable Clock

## Scope

Introduce an `auth.Clock` interface mirroring the existing
`internal/portal/tokens.Clock` shape, thread it through
`MagicLinkHandler`, and keep the production constructor backwards-
compatible. No build tags. No endpoint. This is the pure refactor that
makes the magic-link handler clock-injectable.

## Files

- `internal/portal/auth/magic_link.go` (modified)
- `internal/portal/auth/magic_link_test.go` (modified — add fakeClock
  and one TTL-expiry test that exercises the new injection path)

## Spec

### `internal/portal/auth/magic_link.go`

Add to the top of the file (after the package docs, before
`MagicLinkHandler`):

```go
// Clock is an injectable time source. The default realClock calls
// time.Now().UTC(); tests inject a fakeClock to simulate expiry. The
// shape mirrors internal/portal/tokens.Clock by design — the same
// concrete type can satisfy both interfaces (handy for the e2etest-
// tagged AdvanceableClock).
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }
```

Modify `MagicLinkHandler`:

```go
type MagicLinkHandler struct {
    store     store.Store
    tokensSvc tokens.Service
    sender    senders.Sender
    portalURL string
    clock     Clock
}
```

Refactor constructors so production callers don't change:

```go
// NewMagicLinkHandler constructs a MagicLinkHandler with the real
// system clock. Production callers use this.
func NewMagicLinkHandler(
    s store.Store,
    tokensSvc tokens.Service,
    sender senders.Sender,
    portalURL string,
) *MagicLinkHandler {
    return NewMagicLinkHandlerWithClock(s, tokensSvc, sender, portalURL, realClock{})
}

// NewMagicLinkHandlerWithClock constructs a MagicLinkHandler with the
// supplied clock. Used by unit tests (fakeClock) and the e2etest-
// tagged binary (testclock.AdvanceableClock).
func NewMagicLinkHandlerWithClock(
    s store.Store,
    tokensSvc tokens.Service,
    sender senders.Sender,
    portalURL string,
    clock Clock,
) *MagicLinkHandler {
    return &MagicLinkHandler{
        store:     s,
        tokensSvc: tokensSvc,
        sender:    sender,
        portalURL: portalURL,
        clock:     clock,
    }
}
```

Replace both `time.Now().UTC()` reads in `RequestMagicLink` (line 64)
and `ExchangeMagicLink` (line 105) with `h.clock.Now()`. The current
code is:

```go
now := time.Now().UTC()
```

becomes:

```go
now := h.clock.Now()
```

`realClock.Now()` already returns UTC, so the `.UTC()` strip is correct.

### `internal/portal/auth/magic_link_test.go`

Add a minimal local `fakeClock` (do not import `tokens`'s test-only
`fakeClock`):

```go
type fakeClock struct {
    t time.Time
}

func (f *fakeClock) Now() time.Time           { return f.t }
func (f *fakeClock) advance(d time.Duration)  { f.t = f.t.Add(d) }
```

Add one new test asserting expiry returns `auth.expired_token`:

```go
func TestExchangeMagicLink_ExpiredToken_Returns401WithExpiredCode(t *testing.T) {
    // Build the env manually so we can inject a fakeClock.
    s := openStore(t)
    sender := &captureSender{}
    tokenSvc := tokens.New(s)
    clk := &fakeClock{t: time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC)}
    handler := auth.NewMagicLinkHandlerWithClock(
        s, tokenSvc, sender, "https://portal.example.com", clk)

    fullHandler := &magicLinkOnlyStrict{MagicLinkHandler: handler}
    strictAPI := openapi.NewStrictHandler(fullHandler, nil)

    r := chi.NewRouter()
    r.Post("/api/auth/magic-link/request", strictAPI.RequestMagicLink)
    r.Post("/api/auth/magic-link/exchange", strictAPI.ExchangeMagicLink)
    srv := httptest.NewServer(r)
    t.Cleanup(srv.Close)

    // Issue a token at t=0.
    resp := postJSONBody(t, srv, "/api/auth/magic-link/request",
        map[string]string{"email": "expired@example.com"})
    if resp.StatusCode != http.StatusNoContent {
        t.Fatalf("request: want 204, got %d", resp.StatusCode)
    }
    body := sender.lastBody()
    token := extractTokenFromBody(t, body)

    // Advance the clock past the 15-minute TTL.
    clk.advance(16 * time.Minute)

    // Exchange must fail with auth.expired_token.
    resp2 := postJSONBody(t, srv, "/api/auth/magic-link/exchange",
        map[string]string{"token": token})
    if resp2.StatusCode != http.StatusUnauthorized {
        t.Fatalf("exchange: want 401, got %d", resp2.StatusCode)
    }
    bodyMap := decodeJSONResponse(t, resp2)
    if code, _ := bodyMap["error"].(string); code != "auth.expired_token" {
        t.Errorf("error code: want auth.expired_token, got %q", code)
    }
}
```

(`extractTokenFromBody` is the inline body of the existing
`requestAndExtractToken` helper; either factor it out or duplicate
the index logic — implementor's call.)

## Acceptance criteria

- [ ] `internal/portal/auth/magic_link.go` exports `Clock`, `realClock`,
      `NewMagicLinkHandler`, `NewMagicLinkHandlerWithClock`. The two
      `time.Now().UTC()` calls are replaced with `h.clock.Now()`.
- [ ] `cmd/portal/main.go` compiles unchanged (the existing call to
      `auth.NewMagicLinkHandler` keeps the same signature).
- [ ] Existing magic-link unit tests pass without modification.
- [ ] New test `TestExchangeMagicLink_ExpiredToken_Returns401WithExpiredCode`
      passes.
- [ ] `go build -tags '' ./...` is green.
- [ ] `go vet ./internal/portal/auth/...` clean.
- [ ] `golangci-lint run ./internal/portal/auth/...` clean if the repo
      uses it.

## Production-safety verification

No build tags involved in this story. The whole change is production-
safe: it's a pure refactor that adds an indirection level. `go build
-tags ''` builds the realClock path. The new `Clock` interface and
`realClock` struct are stripped to a couple of bytes by the Go
inliner.

## Notes for the implementer

- Don't rename the existing `now` local variables — keep diffs minimal.
- The `tokens.NewWithClock` constructor exists for the same purpose;
  follow that pattern exactly.
- Don't centralize `Clock` into a shared package. Per-package
  interfaces is the Go-idiomatic shape here.
- Don't introduce a `clockFunc func() time.Time` type alias as a
  shortcut; downstream test stories need the interface name (`Clock`)
  to satisfy with `AdvanceableClock`.
