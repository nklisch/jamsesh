---
id: portal-bearer-middleware-dep-translate
kind: idea
stage: backlog
tags: [portal, auth, error-taxonomy]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# `tokens.BearerMiddleware` swallows DB dep failures as plain `internal` 500

## Idea

`internal/portal/tokens/middleware.go` (`BearerMiddleware`) calls
`svc.Validate(r.Context(), tok)` and on any non-sentinel error falls
through to `httperr.Write(w, r, httperr.ErrInternal(err))`. When the
underlying error is a `pgx` connection failure (transient DB outage),
the response is `{"error":"internal","message":"internal server
error"}` (HTTP 500, no `Retry-After`) instead of the documented
`dep.db_unavailable` (HTTP 503, `Retry-After: 2`).

Surfaced by tightening `tests/e2e/failure/config_and_deps_test.go`
`db_unavailable_via_toxiproxy` to assert on the typed envelope
(story `portal-dep-failure-error-codes-e2e-asserts`): when Toxiproxy
`reset_peer` disrupts Postgres mid-session and the client issues
`GET /api/me`, the middleware's DB lookup fails with `unexpected EOF`
and the response is the generic `internal` 500.

## Why it matters

The `portal-dep-failure-error-codes` feature claims end-to-end coverage
of every dep code in the taxonomy. The DB path through
`BearerMiddleware` violates that claim — operators debugging an outage
see opaque 500s on auth-gated endpoints instead of the
`dep.db_unavailable` they would see on the underlying handler return.

## Proposed fix

Two clean options, both small:

1. **Route the middleware through `WriteFromError`.** Replace
   `httperr.Write(w, r, httperr.ErrInternal(err))` with
   `httperr.WriteFromError(w, r, deperr.WrapDBIfTransient(err))` in the
   `default` branch of the validate-error switch. The wrap helper
   already filters out store-sentinel cases; for the middleware path
   none of those would land in the default branch anyway, so the wrap
   is unconditional.

2. **Translate inline.** Mirror the strict-handler ResponseErrorHandlerFunc
   logic in the middleware: `errors.As(err, &*httperr.Error)` → pass
   through; otherwise `WrapDBIfTransient` + write through
   `WriteFromError`.

Option 1 is the smaller change and keeps the translation centralized.

## Test coverage

Re-enable the assertion in
`tests/e2e/failure/config_and_deps_test.go`
`db_unavailable_via_toxiproxy` that the response is 503 with
`error: dep.db_unavailable` and `Retry-After: 2`. The current
implementation `t.Skip`s that path with a reference to this backlog
item.

## Scope of the gap

Worth a follow-up audit: any other place in the portal that calls
`httperr.Write(w, r, httperr.ErrInternal(err))` directly (rather than
returning the error through the strict handler) bypasses the dep
translator the same way. The middleware is the obvious one; a quick
sweep should catch any others.
