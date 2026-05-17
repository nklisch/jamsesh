---
id: portal-bearer-middleware-dep-translate
kind: story
stage: done
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

## Implementation notes

**Middleware fix.** `internal/portal/tokens/middleware.go` — replaced the
`default` branch of the validate-error switch:

```go
httperr.Write(w, r, httperr.ErrInternal(err))
// →
httperr.WriteFromError(w, r, deperr.WrapDBIfTransient(err))
```

Added the `jamsesh/internal/portal/deperr` import. Business sentinels
(`store.ErrNotFound`, `store.ErrUniqueViolation`) pass through
unchanged because `WrapDBIfTransient` filters them before wrapping;
they then fall through to `ErrInternal` inside `WriteFromError`. Token
sentinels (`ErrInvalidToken`, `ErrExpiredToken`, `ErrRevokedToken`) are
matched in earlier switch arms and never reach the default branch.

**Sweep audit.** Grepped `internal/portal/` for direct
`httperr.Write(... ErrInternal(...))`, `http.Error(...)`, and similar
plain-text emission patterns. Sites found and triaged:

- `internal/portal/tokens/middleware.go` — **FIXED** (this story).
- `internal/portal/githttp/receive_pack.go` lines 81 & 106 — left
  unchanged. These wrap `buildValidationRepo` failure (in-process
  go-git parse error on a pushed pack) and `prereceive.Validator`
  failure. Neither is a DB call; classifying them as `dep.db_unavailable`
  would be misleading. Genuine internal-error 500 is correct.
- `internal/portal/githttp/receive_pack.go` line 90 — already routed
  through `httperr.WriteFromError(w, r, deperr.WrapDBIfTransient(err))`
  for `h.Store.GetSession`; correct.
- `internal/portal/githttp/info_refs.go` line 51,
  `internal/portal/githttp/upload_pack.go` lines 36 & 43,
  `internal/portal/githttp/receive_pack.go` lines 132/139/147 — all
  emit `httperr.ErrGitSubprocessFailed(deperr.WrapGitSubprocess(err))`
  directly via `httperr.Write`. This is the correct envelope already
  (`dep.git_subprocess_failed`, 500, no Retry-After). Going through
  `WriteFromError` would be a no-op since `Write` short-circuits on
  `*httperr.Error`. Left unchanged.
- `internal/portal/auth/middleware.go` lines 45/51/61/66 — every call
  emits `httperr.ErrInsufficientPermission()` on either context misses
  (defensive guards) or `s.GetOrgMember` failures. The story body
  documents the deliberate choice to deny on any error (treats DB
  errors as a missing membership to avoid leaking org existence). Per
  the in-flight conflict note (org-session-invite-policy work touches
  authz semantics), leaving this alone is the right call — changing it
  to a dep envelope would also leak liveness signal to unauthenticated
  callers.
- `internal/portal/githttp/auth.go` lines 48, 87, 110 — `basicAuth`,
  `requireSessionMember`, `checkArchived` write
  `http.Error(w, "internal server error", 500)` directly. **Deferred
  to follow-on** — git's smart-HTTP path doesn't use the JSON envelope
  (clients expect plain text), so converting these requires a separate
  decision about how dep failures should surface in git push/fetch.
  The `httperr` envelope is HTML/JSON-oriented; git clients only
  parse non-200 statuses, so a 503 with `Retry-After` would still be
  meaningful but the body shape is wrong for git tooling. Tracked
  implicitly under the git-http path; not in scope for this story.
- `internal/portal/githttp/auth.go` line 127 (`writeBasicUnauthorized`)
  — correct 401 for unauthenticated git clients (must keep
  `WWW-Authenticate` header). Not affected.
- `internal/portal/wsgateway/gateway.go` lines 145/151/158/171 — the
  WebSocket gateway uses `http.Error` for pre-upgrade rejections.
  **Intentionally skipped** — the WebSocket subprotocol does not
  surface JSON envelopes to browser clients (the upgrade is a one-shot
  HTTP response and browser WS APIs surface the status code, not the
  body). The dep-failure semantics aren't useful here; a 500 from
  membership lookup pre-upgrade is fine. If future WS-client tooling
  starts caring about typed errors this becomes a separate redesign.
- `internal/portal/testclock/handler.go` lines 44/48 — build-tag-gated
  e2e test endpoint; not on the production path. Skipped.
- `internal/portal/auth/oauth.go` — already returns errors through the
  strict handler (returns `deperr.WrapDBIfTransient` / `WrapOAuthProvider`
  from `OauthCallback`). Out of scope; also currently touched by the
  in-flight `portal-oauth-provider-error-taxonomy` story.
- `internal/portal/sessions/handler.go` — touched by another in-flight
  story (`portal-validate-writable-scope-at-create-time`). Not
  inspected here to avoid racing; if relevant gaps exist they'll
  surface through that story's review pass.

**Sweep summary:** 1 site changed (the bearer middleware); 4 site
groups left unchanged because the existing emission is already correct
(genuine programmer-bug 500s on non-DB paths, or already-correct typed
dep envelopes); 3 deferred (git-http basic auth path: format mismatch;
sessions/oauth: in-flight stories own them).

**Test additions.**

- `internal/portal/tokens/middleware_test.go` — added
  `TestBearerMiddleware_TransientDBError_TypedEnvelope` (asserts
  503 + `dep.db_unavailable` + `Retry-After: 2` on a generic
  `errors.New("unexpected EOF")` from `svc.Validate`, the canonical
  pgxpool reset_peer shape) and
  `TestBearerMiddleware_BusinessSentinel_PassesThrough` (asserts that
  `store.ErrNotFound` from a misbehaving Service still falls through
  to `internal` 500, not a dep envelope — guards against future
  over-broadening of `WrapDBIfTransient`).
- `tests/e2e/failure/config_and_deps_test.go` — re-enabled the
  middleware-path assertion in `db_unavailable_via_toxiproxy`. Test
  now mints a real bearer token via `authflow.SignInViaMagicLink`
  BEFORE adding the `reset_peer` toxic, then issues `GET /api/me`
  after disruption and asserts 503 + `dep.db_unavailable` +
  `Retry-After: 2` + `Content-Type: application/json`. The
  follow-up-tracker comment that referenced this story has been
  removed.

**Test runs.**

- `go test ./internal/portal/tokens/...` → pass.
- `go test ./internal/portal/...` → all packages pass.
- `go build ./...` → clean.
- `cd tests/e2e && go build ./...` → clean.
- The Toxiproxy e2e assertion was not executed in this stride
  (requires Docker + the `jamsesh/portal:e2e` image). The unit tests
  cover the contract verbatim; the e2e assertion is now wired so the
  next full e2e run will exercise the live middleware path.

## Review

**Verdict: Approve.**

The middleware fix is the minimal, correct change — option 1 from the
proposal (route through `WriteFromError` + `WrapDBIfTransient`). The
inline comment is appropriately scoped and the import addition is
clean. The earlier switch arms (`ErrExpiredToken`,
`ErrInvalidToken`, `ErrRevokedToken`) correctly short-circuit before
the dep wrap, so token-sentinel semantics are unchanged.

**Test coverage is strong.** Two unit tests pin the contract from
both directions: the transient-shape test asserts the full envelope
(503 + `dep.db_unavailable` + `Retry-After: 2` + JSON content-type)
on a non-sentinel error, and the business-sentinel test guards
against future over-broadening of `WrapDBIfTransient` by pinning
that `store.ErrNotFound` still falls through to `internal` 500. The
re-enabled e2e assertion correctly mints the bearer *before* adding
the toxic — required ordering, easy to get wrong.

**Sweep-audit rigor: high.** Spot-checked three "left unchanged"
sites:
- `auth/middleware.go` — the deny-on-any-error rationale is correct;
  emitting `dep.db_unavailable` here would leak DB liveness to
  unauthenticated callers and conflict with the org-existence
  privacy contract.
- `wsgateway/gateway.go` lines 145/151/158/171 — pre-upgrade
  `http.Error` calls are correct; browser WS clients surface status
  but not body, so the typed envelope offers no value here.
- `githttp/*.go` `ErrGitSubprocessFailed` sites — the envelope is
  already correct and `Write` short-circuits on `*httperr.Error`, so
  rerouting through `WriteFromError` would be a no-op.

**Deferred sites: rationales sound.**
- `githttp/auth.go` — git smart-HTTP clients don't parse JSON
  envelopes; converting requires a separate decision about the
  basic-auth path body format. Reasonable deferral.
- `auth/oauth.go` and `sessions/handler.go` — both touched by
  in-flight stories (`portal-oauth-provider-error-taxonomy` /
  `portal-validate-writable-scope-at-create-time` per recent commits
  `aaf396e` / `87835cc`). Avoiding races is the right call.

**Findings: 0 blockers, 0 important, 0 nits.**

**Test runs (review):**
- `go test ./internal/portal/tokens/... ./internal/portal/...` → all
  pass.
- `go build ./...` → clean.

No parked items.

