---
id: gate-security-revoke-token-bearer-account-check
kind: story
stage: drafting
tags: [security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# `RevokeToken` lets an authenticated caller revoke any token they happen to know

## Severity
Medium

## Domain
Authentication & Authorization

## Location
`internal/portal/tokens/handlers.go:58-80`,
`internal/portal/tokens/service_impl.go:177-197`

## Evidence
```go
func (h *Handler) RevokeToken(ctx context.Context, req openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
    _, ok := AccountFromContext(ctx)
    ...
    revokeAll := req.Body.RevokeAll
    if err := h.svc.Revoke(ctx, req.Body.Token, revokeAll); err != nil {
```

`service_impl.go:178-196` then looks up the row by hash of the body token
and revokes all tokens for **that row's AccountID** — not the bearer's.
So caller A authenticated as A but submitting
`{token: <B's leaked token>, revoke_all: true}` revokes every token for
B. A leaked or low-trust short-lived fetch token of B gives any signed-in
user the ability to mass-revoke B's sessions.

## Remediation direction
Either (a) ignore `req.Body.Token` and always revoke based on the
bearer-authenticated account, or (b) require that
`hashToken(req.Body.Token)`'s AccountID equals the caller's account ID
and return 403 otherwise.
