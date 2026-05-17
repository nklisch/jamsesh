---
id: portal-dep-failure-error-codes-db
kind: story
stage: done
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

## Implementation notes

Surgical wrap-and-go pass applying `deperr.WrapDBIfTransient` to every
handler error path that bubbles a non-sentinel store error to the
strict-handler boundary. The wrap is `errors.Is`-safe for
`store.ErrNotFound` and `store.ErrUniqueViolation`, so existing 404/409
branches are untouched.

**Scope.** 95 wrap sites across 19 production files in
`internal/portal/`:

- `accounts/handlers.go` (3), `accounts/orgs.go` (4)
- `sessions/handler.go` (10), `sessions/listing.go` (2),
  `sessions/files.go` (3), `sessions/invites.go` (6),
  `sessions/members.go` (3), `sessions/refmodes.go` (4),
  `sessions/state.go` (10)
- `comments/handlers.go` (7)
- `finalize/lock_acquire.go` (7), `finalize/lock_patch.go` (4),
  `finalize/lock_release.go` (3), `finalize/plan.go` (4),
  `finalize/fetch_token.go` (2), `finalize/mark_shipped.go` (5)
- `auth/magic_link.go` (4), `auth/oauth.go` (3 — DB parts only;
  provider-exchange wrap landed in story `oauth`)

(Plus 1 prior `WrapDBIfTransient` in `githttp/receive_pack.go` from the
git-subprocess sibling, untouched here.)

**Intentionally skipped.**

- `comments/service.go` — service-layer DB errors bubble up via the
  handler's existing `fmt.Errorf("comments: ...: %w", err)` wrap, which
  the handler-level `WrapDBIfTransient` then classifies. Wrapping
  inside the service would double-wrap and obscure the chain. Matches
  the design's "outer return is where the dep classification matters"
  rule.
- `events/log.go`, `automerger/*.go`, `postreceive/*.go` — background-
  worker DB calls with no HTTP surface, explicitly out of scope per
  the parent feature's "what's explicitly out of scope" section.
- `mcpendpoint/*.go` — MCP tool errors flow through the MCP SDK's own
  envelope, not the strict-handler translator. Out of scope per the
  feature design (no MCP code in the taxonomy).
- Non-DB error sites in audited files (git refs iteration, go-git
  open/read, JSON marshal/unmarshal, random-byte generation, URL
  parse, `storage.CreateRepo` disk write): left as plain
  `fmt.Errorf(...)`, which the translator routes to the existing
  `ErrInternal` (500) path. These are not dep failures.
- `auth/provision.go`, `auth/slug.go` — store calls inside these are
  invoked from `magic_link` and `oauth` handlers and accounts handlers
  whose outer return paths already wrap with `WrapDBIfTransient`.
  Wrapping inside the helpers would double-wrap; sentinel chain is
  preserved by `errors.Is`.

**Tests.** Four representative HTTP-level dep-failure tests added,
each stubbing a single store method to return `errors.New("conn refused")`
and asserting the response is `503` with `Retry-After: 2` and
`{error: "dep.db_unavailable"}`:

- `accounts/handlers_test.go`: `TestGetMe_DBUnavailable_Returns503DepDBUnavailable`
- `sessions/listing_state_test.go`: `TestListSessions_DBUnavailable_Returns503DepDBUnavailable`
- `comments/service_test.go`: `TestHandlerListComments_DBUnavailable_Returns503DepDBUnavailable`
- `finalize/lock_release_test.go`: `TestReleaseFinalizeLock_DBUnavailable_WrapsAsDepDB`
  — the finalize tests call the handler directly (no HTTP); this
  asserts `errors.Is(err, deperr.ErrDB)` to prove the wrap is in place
  for the production translator to consume.

The accounts and comments test envs were extended with
`newAccountsTestEnvWithStore` / `newTestEnvWithStore` helpers so a
wrapping store can be injected while keeping the original test
helpers backwards-compatible. The strict-handler translator
(`httperr.WriteFromError`) is now wired explicitly in the comments
test env, matching `cmd/portal/main.go`'s production wiring.

**Verification.**

- `go build ./...` clean
- `go vet ./internal/portal/...` clean
- `go test ./internal/portal/...` all packages pass (44 packages,
  100% green on `-count=1`)
- `go test ./...` repo-wide pass

No existing tests required updates: pre-existing 500-asserting tests
target non-DB failure modes (storage disk failures, go-git open
failures) that correctly remain plain 500s.

## Review

**Verdict.** Approve.

Spot-checked five representative production files (`accounts/handlers.go`,
`sessions/handler.go`, `comments/handlers.go`,
`finalize/lock_acquire.go`, `auth/oauth.go`): every wrap site puts the
business-sentinel branch (`errors.Is(err, store.ErrNotFound)` /
`store.ErrUniqueViolation`) ahead of the `WrapDBIfTransient` return,
preserving the existing 404 / 409 envelopes. Transaction wraps go on
the outer `txErr` site, matching the design's "outer return is where
the dep classification matters" rule (`sessions/handler.go:95`,
`accounts/orgs.go:197`). The `RequireSessionMember` / `RequireOrgMember`
"hard error" path is wrapped (e.g. `comments/handlers.go:38`,
`sessions/handler.go:57,142,297`) so a DB blip during the membership
guard still surfaces as `dep.db_unavailable` rather than a bare 500.

Reviewed the four new dep-failure tests
(`accounts/handlers_test.go`, `sessions/listing_state_test.go`,
`comments/service_test.go`, `finalize/lock_release_test.go`). Each
injects a real `failingStore` double, validates the upstream auth path,
and asserts the right slice of the contract — HTTP-level tests check
`{503, Retry-After: 2, error: dep.db_unavailable}`; the finalize
handler-direct test asserts `errors.Is(err, deperr.ErrDB)` since
finalize uses a handler-call shape rather than HTTP-roundtrip. The two
`newAccountsTestEnvWithStore` / `newTestEnvWithStore` injection
helpers were extended carefully — they wire `httperr.WriteFromError`
explicitly to mirror `cmd/portal/main.go` so the translator runs
under test.

Verified the comments/service.go skip: every `fmt.Errorf` inside the
service is consumed by the handler's outer `deperr.WrapDBIfTransient`
in `comments/handlers.go:38,100,135,169,231`. No double-wrap.

Repo-wide audit (`grep fmt.Errorf` minus the documented skip list)
shows only non-DB sites remaining — disk failures (`storage/`,
`sessions/files.go` blob reads), git open/resolve (`finalize/plan.go`),
provider transport errors in `oauth/github.go`, nonce/rand reads,
config validation, prereceive scope parsing. All correctly remain
plain 500s per design.

Build, vet, and `go test ./internal/portal/...` all green across 25
packages.

**Skip list assessment.** Reasonable and consistent with the parent
feature's out-of-scope policy. `events/log.go`, `automerger`, and
`postreceive` are background workers with no HTTP surface;
`mcpendpoint` flows through the MCP SDK envelope; `auth/provision.go`
and `auth/slug.go` are called from outer-wrapped handlers (verified
via `accounts/handlers.go:88` and `auth/oauth.go:149`); `githttp/`
already had its own `WrapDBIfTransient` from the git-subprocess
sibling. Service-layer skip is sound — wrapping inside `Service.List`
would double-wrap when the handler's outer `WrapDBIfTransient` runs.

**Findings.** None.

- Blockers: 0
- Important: 0
- Nits: 0

No parked items.

Advancing review → done.
