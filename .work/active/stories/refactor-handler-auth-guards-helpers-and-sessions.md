---
id: refactor-handler-auth-guards-helpers-and-sessions
kind: story
stage: implementing
tags: [refactor, portal]
parent: refactor-handler-auth-guards
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Auth-Guards — Define helpers and migrate sessions handler

Define the `handlerauth` package and migrate `internal/portal/sessions/handler.go`
as the first consumer. This story proves the helper shape against the most
complex consumer (4 handlers, all three guard tiers represented).

## Files

- New: `internal/portal/handlerauth/handlerauth.go`
- New: `internal/portal/handlerauth/handlerauth_test.go`
- Modify: `internal/portal/sessions/handler.go` (4 handlers + helper imports)

## Current state (per parent feature body)

Each of CreateSession, PatchSession, FinalizeSession, AbandonSession
duplicates account extraction (8 LoC) and org/session membership checks
(12-15 LoC each).

## Target state

`handlerauth.RequireAccount`, `RequireOrgMember`, `RequireSessionMember` plus
`AuthFail` per parent body. All four sessions handlers reduced to a single
guard call each.

## Implementation notes

- Place the package at `internal/portal/handlerauth`; do not extend `tokens`
  because `tokens` is auth-only (parsing/middleware), not authorization.
- The `AuthFail` struct contains *both* the 401 and 403 payloads as fields;
  only one is populated based on `Status`. Avoids `interface{}` and keeps
  call sites type-safe.
- Build the typed wrapping in each handler with a tiny per-handler helper
  inside `sessions/handler.go`, e.g.:

  ```go
  func patchSessionFail(f handlerauth.AuthFail) openapi.PatchSessionResponseObject {
      if f.Status == 401 {
          return openapi.PatchSession401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
      }
      return openapi.PatchSession403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
  }
  ```

  This keeps the operation-specific typing local to the handler package.

## Acceptance

- [ ] `go build ./...` passes
- [ ] `go test ./internal/portal/sessions/... ./internal/portal/handlerauth/...` passes
- [ ] `internal/portal/sessions/handler.go` total LoC drops by ~80
- [ ] Zero direct calls to `store.GetOrgMember` / `store.GetSessionMember` remain
      in `internal/portal/sessions/handler.go` (those calls now live behind
      the helper package)
- [ ] Helper has unit tests for: nil-account-in-context, valid-account, valid
      account + missing org member, valid account + missing session member

## Risk

LOW. Each handler's tests already pin the response shape; rerunning them
catches any regression.

## Rollback

`git revert` the commit. Single-file changes are easy to reverse.
