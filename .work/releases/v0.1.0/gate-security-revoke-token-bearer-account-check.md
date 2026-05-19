---
id: gate-security-revoke-token-bearer-account-check
kind: story
stage: done
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

## Implementation notes

Chose option (b): require AccountID match so callers retain the ability to
revoke a specific token by value, while closing the cross-account vector.

Changes:

- **`docs/openapi.yaml`**: Added `403` response to `revokeToken` endpoint,
  then ran `make generate-api-go` to emit `RevokeToken403JSONResponse` in
  the generated server code.

- **`internal/portal/tokens/service.go`**: Added `ErrForbidden` sentinel.
  Updated `Service.Revoke` signature to accept `callerAccountID string` as
  the second argument (before `rawToken`).

- **`internal/portal/tokens/service_impl.go`**: After the store lookup,
  compare `row.AccountID` to `callerAccountID`; return `ErrForbidden` if
  they differ. This check fires before any mutation, so the attacker's
  request leaves the victim's tokens untouched.

- **`internal/portal/tokens/handlers.go`**: Extract the bearer account via
  `AccountFromContext` (already present but previously unused after the
  guard), pass it to `svc.Revoke`, and map `ErrForbidden` → 403 with
  `auth.forbidden` / "token does not belong to the authenticated account".

- **`internal/portal/tokens/service_test.go`**: Updated all existing
  `svc.Revoke(...)` call sites to supply the owner's account ID. Added two
  new tests:
  - `TestService_Revoke_CrossAccount_Single` — single-token path rejects
    with `ErrForbidden` and leaves victim token intact.
  - `TestService_Revoke_CrossAccount_All` — revoke-all path rejects with
    `ErrForbidden` and leaves both victim tokens intact.

- **`internal/portal/tokens/basic_test.go`** and
  **`internal/portal/tokens/middleware_test.go`**: Updated `Revoke` call
  site and mock signature to match new interface.

Build: `go build ./...` — clean. Tests: `go test ./internal/portal/tokens/...` — all pass.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Cross-account revoke vector closed. Service.Revoke now takes callerAccountID and rejects with ErrForbidden when row.AccountID mismatches the bearer. Handler maps ErrForbidden to RevokeToken403JSONResponse (auth.forbidden). openapi.yaml extended with 403 on revokeToken; api gen refreshed. Two unit tests (single-token + revoke_all) confirm victim tokens remain valid when A attempts revoke of B's token. Existing 38 tokens-package tests all pass.
