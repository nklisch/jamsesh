# Strict-Server Partial Handler Shim with Panic-Stubs

Each per-package handler test file that needs to mount its handler at
HTTP-router level (rather than calling the strict-server method directly)
defines a `type <pkg>OnlyStrict struct { *<pkg>.Handler }` (or
`<pkg>OnlyHandler`) wrapper whose declared receiver methods cover every
other operation in `openapi.StrictServerInterface` and `panic("not wired")`.
The real handler's exported methods are inherited through struct embedding;
only the methods owned by other packages are panicked out.

## Rationale

`internal/api/openapi/server.gen.go` exposes one giant
`StrictServerInterface` covering every REST operation across every domain
(accounts, sessions, tokens, comments, finalize, playground, auth...). Tests
for one package only wire that package's handler, so the embedded `*Handler`
satisfies its own methods and the explicit `panic("not wired")` stubs
satisfy the remaining interface methods. The panic is intentional — it
converts "test accidentally hit an unwired endpoint" from a quiet `nil
pointer deref` panic into a loud "not wired" panic with a diagnostic stack.
The naming convention `<pkg>OnlyStrict` makes the intent visible at the
struct definition.

## Examples

### Example 1: playground handler test shim

**File**: `internal/portal/playground/handler_test.go:89`

```go
type playgroundOnlyStrict struct {
    *playground.Handler
}

func (h *playgroundOnlyStrict) ExchangeMagicLink(_ context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
    panic("not wired")
}
func (h *playgroundOnlyStrict) RequestMagicLink(_ context.Context, _ openapi.RequestMagicLinkRequestObject) (openapi.RequestMagicLinkResponseObject, error) {
    panic("not wired")
}
// ... ~80 more not-wired stubs for every other StrictServerInterface method
```

### Example 2: sessions handler test shim

**File**: `internal/portal/sessions/handler_test.go:91`

```go
type sessionsOnlyStrict struct {
    *sessions.Handler
}

func (h *sessionsOnlyStrict) ExchangeMagicLink(_ context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
    panic("not wired")
}
// ... etc
```

### Example 3: accounts handler test shim with explicit doc-comment

**File**: `internal/portal/accounts/handlers_test.go:84`

```go
// accountsOnlyStrict wraps accounts.Handler and panics on methods it doesn't own.
type accountsOnlyStrict struct {
    *accounts.Handler
}

func (a *accountsOnlyStrict) CreatePlaygroundSession(_ context.Context, _ openapi.CreatePlaygroundSessionRequestObject) (openapi.CreatePlaygroundSessionResponseObject, error) {
    panic("not wired")
}
```

Replicated in 8 packages: `accounts/handlers_test.go:84`,
`auth/magic_link_test.go:111`, `auth/oauth_test.go:51`,
`comments/service_test.go:42`, `playground/handler_test.go:89`,
`sessions/handler_test.go:91`, `tokens/handlers_test.go:29` (named
`tokensOnlyHandler`), `wsgateway/ticket_handler_test.go:34` (named
`wsTicketOnlyHandler`). 244 `panic("not wired")` lines across these files.

## When to Use

- A handler test needs to mount its handler at the chi-router level (so it
  can exercise the openapi `HandlerFromMux` wiring + middleware) and only
  its own package's methods need real implementations.
- The package under test owns a contiguous slice of `StrictServerInterface`
  methods, with the rest belonging to other packages.

## When NOT to Use

- The test calls the package's strict-server method directly (e.g.
  `handler.CreateSession(ctx, req)`). No shim is needed; the embedded
  handler IS the StrictServerInterface contributor.
- All of `StrictServerInterface` is in scope (cross-package integration test
  in `tests/e2e/`). Use the real composed StrictServer from
  `cmd/portal/main.go`.

## Common Violations

- Using `return nil, nil` instead of `panic("not wired")` — a silent
  zero-value response can produce confusing 200 OKs with empty bodies that
  look like passing tests.
- Defining only some of the not-wired methods and relying on an embedded
  zero `*Handler` field to nil-deref the rest — the embedded handler does
  implement them, but with the wrong package's logic.
- Renaming convention (e.g. `<pkg>StrictShim`, `<pkg>FakeHandler`) — keep
  the `<pkg>OnlyStrict` / `<pkg>OnlyHandler` form so grep across the repo
  turns up every instance at once.
