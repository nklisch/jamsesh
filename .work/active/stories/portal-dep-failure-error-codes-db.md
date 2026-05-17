---
id: portal-dep-failure-error-codes-db
kind: story
stage: implementing
tags: [portal]
parent: portal-dep-failure-error-codes
depends_on: [portal-dep-failure-error-codes-envelope-helper]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Wire DB dep failures to `dep.db_unavailable`

Applies the `deperr.WrapDBIfTransient` discipline to every handler
error path that returns a non-sentinel store error, so DB connection
failures, query timeouts, and pgx/sqlite I/O errors surface as
`{error: "dep.db_unavailable"}` 503 with `Retry-After: 2`.
Business sentinels (`store.ErrNotFound`, `store.ErrUniqueViolation`)
are explicitly preserved as 404/409.

## Scope (handler files to audit + wrap)

Each file below has multiple `h.store.<Query>(...)` call sites; the
audit applies the same pattern at each `return nil, err` (or
`return nil, fmt.Errorf("...%w", err)`) site that follows a store call.

- `internal/portal/accounts/handlers.go` — `GetMe`, `CreateOrg`
- `internal/portal/accounts/orgs.go` — `ListOrgMembers`,
  `CreateOrgInvite`, `AcceptOrgInvite`
- `internal/portal/sessions/handler.go` — `GetSession`, `PatchSession`,
  `FinalizeSession`, `AbandonSession`
- `internal/portal/sessions/listing.go` — `ListSessions`
- `internal/portal/sessions/files.go` — `GetSessionFile`
- `internal/portal/sessions/invites.go` — `InviteToSession`,
  `AcceptSessionInvite`
- `internal/portal/sessions/members.go` — `RemoveSessionMember`
- `internal/portal/sessions/refmodes.go` — `UpsertRefMode`
- `internal/portal/sessions/state.go` — `ListSessionRefs`,
  `GetSessionDigest`
- `internal/portal/comments/handlers.go` — `ListComments`,
  `CreateComment`, `ResolveComment`
- `internal/portal/comments/service.go` — service-layer DB calls
- `internal/portal/finalize/lock_acquire.go`,
  `lock_patch.go`, `lock_release.go`, `lock_check.go`
- `internal/portal/finalize/plan.go`
- `internal/portal/finalize/fetch_token.go`
- `internal/portal/finalize/mark_shipped.go`
- `internal/portal/finalize/membership.go`
- `internal/portal/auth/magic_link.go` — `ExchangeMagicLink` (DB
  parts only; SMTP wrap is in story 2)
- `internal/portal/auth/oauth.go` — `OauthCallback` (DB parts only;
  OAuth provider wrap is in story 4)
- `internal/portal/auth/provision.go` — `FindOrProvision`'s store
  errors

## Pattern

Today:

```go
sess, err := h.store.GetSession(ctx, orgID, sessionID)
if err != nil {
    if errors.Is(err, store.ErrNotFound) {
        return openapi.GetSession404JSONResponse{...}, nil
    }
    return nil, fmt.Errorf("get session: %w", err)
}
```

Target:

```go
sess, err := h.store.GetSession(ctx, orgID, sessionID)
if err != nil {
    if errors.Is(err, store.ErrNotFound) {
        return openapi.GetSession404JSONResponse{...}, nil
    }
    return nil, deperr.WrapDBIfTransient(fmt.Errorf("get session: %w", err))
}
```

The `WrapDBIfTransient` is a safety net: if a caller forgets to
branch on `ErrNotFound` first, the helper preserves the sentinel
chain via `errors.Is`. The translator in
`httperr.WriteFromError` then ignores the wrap because
`store.ErrNotFound` is not a `deperr.ErrDB` (the unconditional path),
and falls through to `ErrInternal` — at which point the missing 404
branch is a bug the audit catches.

**Important nuance.** For sites that wrap many calls inside a single
`WithTx` callback, wrap at the outer `err` site so the inner error
chain is preserved. Don't wrap inside the `tx.WithTx` callback — the
outer return is where the dep classification matters.

## Files (test updates)

- Existing unit tests that asserted plain-text 500 on a DB failure
  (search: `t.Run("...db..."` or `TestXxx_DBError_*` patterns) update
  their assertions to expect the typed envelope.
- Add new dep-failure unit tests where coverage was thin. Suggested
  targets (audit-driven; not exhaustive):
  - `accounts/handlers_test.go`: GetMe with a store that returns
    `errors.New("conn refused")` -> 503 + `dep.db_unavailable`
  - `sessions/listing_state_test.go`: ListSessions same shape
  - `comments/handlers_test.go`: ListComments same shape

  Implement these as table-driven tests using a `failingStore` test
  double that returns `errors.New("conn refused")` from the relevant
  method.

## Audit method

```bash
grep -rn '"%w", err' internal/portal/ \
  | grep -v _test.go \
  | grep -v deperr.Wrap
```

Anything left is a candidate. Per the design, **only wrap when a
store-call error has been reached** — not for, say, JSON marshal
failures or in-process errors. The audit must be call-site-aware, not
blanket.

## Acceptance criteria

- [ ] Every handler that touches `h.store.<X>` wraps non-sentinel
      errors with `deperr.WrapDBIfTransient` (or `WrapDB` where no
      business sentinel is possible)
- [ ] `store.ErrNotFound` paths continue to return their existing 404
      envelopes
- [ ] `store.ErrUniqueViolation` paths continue to return their
      existing 409 envelopes
- [ ] DB-disrupted unit tests assert on `{error: "dep.db_unavailable",
      status: 503, Retry-After: "2"}`
- [ ] `go build ./...` clean
- [ ] `go test ./internal/portal/...` passes

## Test approach

Add a shared `failingStore` test helper in
`internal/portal/testutil/` (or similar — audit existing test
helpers first; the project may have an `_test.go`-internal pattern
already). The helper returns a configurable error from a configured
method.

For each handler family, add at least one test:
`TestXxx_DBUnavailable_Returns503DepDBUnavailable`.

## Risk

MEDIUM. Touches ~100 call sites across many files. The wrap is
mechanical but easy to miss one. Mitigation: the `errors.Is`-based
translator means a missed wrap *degrades* to today's behavior (plain
500), not a crash. The e2e story (7) catches the worst misses.

Cruft watch: do NOT introduce defensive `if err != nil` shims that
weren't there before. Wrap inline where the existing `if err != nil`
already exists.

## Rollback

`git revert`. The translator and existing 404/409 paths are
independent; nothing requires a coordinated rollback.
