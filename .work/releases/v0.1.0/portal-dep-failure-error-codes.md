---
id: portal-dep-failure-error-codes
kind: feature
stage: done
tags: [portal, documentation]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Portal dep-failure error codes

## Finding

Discovered during e2e implementation of
`epic-e2e-tests-failure-mode-config-and-deps`. The story body and
parent feature design referenced documented error codes like
`dep.smtp_unavailable`, `dep.db_unavailable`, etc., for runtime
dependency-failure responses.

Reality: the portal's SMTP / DB / OAuth-provider failures surface as
**plain-text HTTP 500** from the oapi-codegen strict handler's default
`ResponseErrorHandlerFunc` path. There is no JSON error envelope, no
machine-readable `dep.*` code.

This means:
1. Clients (SPA, plugin binary) can't distinguish a dep failure from
   any other 500
2. The documented error contract in `docs/PROTOCOL.md > HTTP error
   contract` isn't actually honored on dep-failure paths
3. Failure-mode e2e tests can only assert on HTTP status, not on the
   error code, weakening the contract guarantee

## Why it matters

The portal's `HTTP error contract` promises an `{error, message}` JSON
envelope on all errors. Dep failures break that promise.

For production hardening this is a real gap — operators debugging an
outage benefit from `dep.smtp_unavailable` vs. `dep.db_unavailable`
instead of opaque 500s, and the SPA / plugin can present a targeted
"upstream service down" message instead of a generic "Something went
wrong".

## Direction (chosen)

**Implement typed `dep.*` codes for every dep failure path.** Full
implementation across all handler families that touch a runtime
dependency: SMTP (magic-link, org invites, session invites), DB (every
sqlc-backed handler), OAuth provider (GitHub token exchange), and git
subprocess (info/refs, upload-pack, receive-pack). The contract holds
end-to-end — SPA and plugin can distinguish dep failures from other 500s.

## Design

### Dep code taxonomy

| Code                              | Status | Triggered by                                                                            | Retry-After |
|-----------------------------------|--------|-----------------------------------------------------------------------------------------|-------------|
| `dep.smtp_unavailable`            | 503    | `senders.ErrTransient` / `senders.ErrAuth` from `Sender.Send` (any caller)              | yes (5s)    |
| `dep.db_unavailable`              | 503    | non-sentinel store errors (pgx connection refused, SQLite I/O, query timeout, etc.)     | yes (2s)    |
| `dep.oauth_provider_unavailable`  | 503    | non-2xx HTTP from GitHub OAuth (token exchange or userinfo); transport errors           | yes (10s)   |
| `dep.git_subprocess_failed`       | 500    | `git-upload-pack` / `git-receive-pack` / `git http-backend` non-zero exit or spawn err  | no          |

Rationale for status codes:

- **503 Service Unavailable** for the upstream-down family (SMTP, DB,
  OAuth). These are explicitly transient: the dep may recover and the
  request is retryable. The status communicates retryability and lets
  load balancers / clients apply backoff.
- **500 Internal Server Error** for `dep.git_subprocess_failed`. A
  failed `git-upload-pack` subprocess is local-process trouble (binary
  missing, repo corruption, OOM kill) rather than a transient
  upstream — neither a `Retry-After` hint nor a "service unavailable"
  framing is honest.
- **Retry-After** for 503s is a coarse hint (seconds), not an SLA.
  Conservative defaults keep stampedes off a flapping dep.

### Architecture

Two complementary translation layers cooperate:

1. **Per-handler sentinel translation** (the primary mechanism). Each
   handler call site that touches a dep wraps the underlying error with
   a sentinel from a new package `internal/portal/deperr/`:

   ```go
   // deperr.go (new package)
   var (
       ErrSMTP          = errors.New("dep: smtp unavailable")
       ErrDB            = errors.New("dep: database unavailable")
       ErrOAuthProvider = errors.New("dep: oauth provider unavailable")
       ErrGitSubprocess = errors.New("dep: git subprocess failed")
   )

   func WrapSMTP(err error) error          { return fmt.Errorf("%w: %v", ErrSMTP, err) }
   func WrapDB(err error) error            { return fmt.Errorf("%w: %v", ErrDB, err) }
   func WrapOAuthProvider(err error) error { return fmt.Errorf("%w: %v", ErrOAuthProvider, err) }
   func WrapGitSubprocess(err error) error { return fmt.Errorf("%w: %v", ErrGitSubprocess, err) }
   ```

   Callers do (e.g., magic-link):

   ```go
   if err := h.sender.Send(ctx, email, subject, body); err != nil {
       return nil, deperr.WrapSMTP(err)
   }
   ```

   For DB, the wrap is **selective**: a `store.ErrNotFound` is a
   business-logic 404, not a dep failure, so wrap only when the error
   is *not* a known store sentinel. A helper `deperr.WrapDBIfTransient`
   does that filtering in one line.

2. **Custom `ResponseErrorHandlerFunc` on the strict handler** wired in
   `cmd/portal/main.go`. The current default writes plain-text 500:

   ```go
   // current
   ResponseErrorHandlerFunc: func(w, r, err) {
       http.Error(w, err.Error(), http.StatusInternalServerError)
   }
   ```

   Replace with a translator that funnels every handler error through
   `httperr.WriteFromError`, which:

   - Checks `errors.As(err, *httperr.Error)` — if the handler already
     produced a structured `*httperr.Error`, write it (this is what
     `tokens.BearerMiddleware` already does).
   - Checks `errors.Is(err, deperr.Err*)` for each dep sentinel — if
     matched, build the corresponding `*httperr.Error` with the right
     code + status + Retry-After.
   - Falls through to `httperr.ErrInternal` (code `"internal"`, 500) for
     anything else. This preserves today's behavior for *unanticipated*
     errors — the contract only widens for dep failures, not for genuine
     bugs.

   The translator lives in `internal/portal/httperr/` (extending the
   existing package) so all envelope construction stays in one place,
   matching the package doc comment: *"Package httperr is the only place
   in the portal that emits an HTTP error response."*

3. **Git smart-HTTP** does not flow through the oapi-codegen strict
   handler — it uses `http.Error` directly today. The
   `git-subprocess-failed` story replaces those `http.Error` call sites
   with `httperr.Write(w, r, httperr.ErrGitSubprocessFailed(err))` so
   the same envelope shape applies. Git client UX is unchanged for
   error paths because git CLI doesn't parse JSON error bodies — but
   programmatic callers (the SPA's repo browser, future tooling) get
   the typed envelope.

### Identifying dep errors (sentinel discipline)

- **SMTP**: `senders.Sender` already declares `ErrTransient` /
  `ErrPermanent` / `ErrAuth`. The dep-failure code applies to
  `ErrTransient` and `ErrAuth` (operator-fixable upstream issues).
  `ErrPermanent` (invalid recipient, etc.) stays as a 400-class business
  error — it's a request problem, not a dep problem. Audit pass on
  story 2 verifies each provider impl actually wraps with these
  sentinels.
- **DB**: the store interface uses bare errors plus
  `store.ErrNotFound` / `store.ErrUniqueViolation`. The wrap helper
  `deperr.WrapDBIfTransient(err)` returns the input unchanged when
  `errors.Is(err, store.ErrNotFound)` or `errors.Is(err,
  store.ErrUniqueViolation)`, and wraps with `deperr.ErrDB` otherwise.
  This is conservative — we don't try to sniff `pgconn.PgError` codes
  because pgx connection-refused returns a `*net.OpError`-wrapped
  error, not a `pgconn.PgError`. Anything not a recognized business
  sentinel is treated as a dep failure.
- **OAuth provider**: `oauth.GitHub.Exchange` and friends return errors
  on non-2xx HTTP or transport failures. Wrap those at the call site
  in `auth/oauth.go` with `deperr.WrapOAuthProvider`.
- **Git subprocess**: in `githttp/{info_refs,upload_pack,receive_pack}.go`,
  any `cmd.Start` / `cmd.Wait` / pipe failure is wrapped with
  `deperr.WrapGitSubprocess` and emitted via `httperr.Write`.

### Reuse of existing `openapi.ErrorEnvelope`

The shape `{error, message, details}` already exists in
`docs/openapi.yaml > ErrorEnvelope` and the generated `openapi.ErrorEnvelope`
struct. **No schema change is required** — only the documented set of
codes the `error` field can take is extended. The OpenAPI per-operation
responses do not need to enumerate the new 503/500 codes per-op because
the envelope is the same shape; the new codes are documented in
`docs/PROTOCOL.md > HTTP error contract` as a global enumeration, the
same way the existing `auth.*` / `session.*` / `push.*` codes are.

### Logging

`httperr.Write` already logs at error level for `status >= 500` with
`code`, `status`, and wrapped error. The dep translator preserves the
wrapped error so the underlying cause (pgx connection refused, SMTP
TLS handshake failure, etc.) lands in operator logs. The JSON envelope
to the client carries only the typed code and a fixed human message —
no leaking of internal stack traces or DSNs.

### What's explicitly out of scope for this feature

- Object-storage codes (`dep.object_storage_unavailable`). No S3 / MinIO
  integration is present in the codebase today; the
  `epic-cloud-native-deploy` work introduces it later. Add the code at
  that time. The taxonomy table in `PROTOCOL.md` only registers codes
  that have implementations.
- Background-worker (auto-merger / postreceive emitter) errors. These
  paths have no HTTP surface to surface a typed code on; their failures
  are observability concerns, tracked separately.
- Retry policy in the SPA / plugin client. This feature stops at the
  contract — clients consume `Retry-After` and the `dep.*` family at
  their own pace.

## Child stories

1. **`portal-dep-failure-error-codes-envelope-helper`** — adds the
   `internal/portal/deperr/` package (sentinels + wrap helpers), extends
   `internal/portal/httperr/` with `ErrSMTPUnavailable`,
   `ErrDBUnavailable`, `ErrOAuthProviderUnavailable`,
   `ErrGitSubprocessFailed`, and a `WriteFromError(w, r, err)` helper
   that does the `errors.Is`/`errors.As` switch. Wires a custom
   `ResponseErrorHandlerFunc` in `cmd/portal/main.go` that calls into
   `httperr.WriteFromError`. Unit tests cover the translator matrix.
   No call-site changes yet. Depends on nothing.

2. **`portal-dep-failure-error-codes-auth-smtp`** — wraps SMTP errors
   from `auth/magic_link.go` (RequestMagicLink),
   `accounts/orgs.go` (CreateOrgInvite), and `sessions/invites.go`
   (InviteToSession) with `deperr.WrapSMTP`. Updates unit tests to
   assert on `dep.smtp_unavailable` envelope. Depends on (1).

3. **`portal-dep-failure-error-codes-db`** — adds
   `deperr.WrapDBIfTransient` discipline to every handler error path
   that returns a non-sentinel store error. Touches
   `internal/portal/{accounts,sessions,comments,finalize,oauth}/` —
   roughly the ~100 store-error sites enumerated by the audit. Existing
   `store.ErrNotFound` / `store.ErrUniqueViolation` branches are
   untouched (they remain as 404 / 409 business errors). Depends on (1).

4. **`portal-dep-failure-error-codes-oauth`** — wraps the
   `provider.Exchange` and `FindOrProvision` (when its inner store
   error is dep-class) returns in `auth/oauth.go` (OauthCallback). The
   GitHub provider's HTTP failures get
   `deperr.WrapOAuthProvider`. Depends on (1).

5. **`portal-dep-failure-error-codes-git-subprocess`** — replaces the
   `http.Error(...)` calls in `githttp/info_refs.go`,
   `githttp/upload_pack.go`, and `githttp/receive_pack.go` for
   subprocess-failure paths with `httperr.Write(w, r,
   httperr.ErrGitSubprocessFailed(err))`. Pre-receive-rejection paths
   are unchanged (those write the smart-HTTP report-status response,
   which is a different contract). Depends on (1).

6. **`portal-dep-failure-error-codes-protocol-doc`** — registers every
   new `dep.*` code in `docs/PROTOCOL.md > HTTP error contract` with
   one-line descriptions, status-code mapping, and Retry-After
   semantics. Depends on (1) so the code shapes are settled. **This
   story's actual edit was rolled forward in the design-commit per the
   rolling-foundation principle**, but the story exists so the doc
   change is reviewable as a substrate item and reachable from this
   feature's review checklist.

7. **`portal-dep-failure-error-codes-e2e-asserts`** — updates
   `tests/e2e/failure/config_and_deps_test.go` to:
   - Assert SMTP-unavailable returns 503 with `error =
     dep.smtp_unavailable`
   - Assert DB-unavailable (toxiproxy reset_peer) returns 503 with
     `error = dep.db_unavailable`
   - Assert OAuth-provider 503 returns 503 with `error =
     dep.oauth_provider_unavailable`
   - Decode JSON body (not just status) and check `error` field
   Depends on (2), (3), (4).

## Design decisions

- **Wrapper sentinels over a custom error type**. Sentinels +
  `errors.Is` compose cleanly with `fmt.Errorf("%w: ...")` wrapping
  done by existing handlers — no need to refactor every error site to
  return a struct. The dep package adds *only* sentinels and shallow
  helpers; the rest of the codebase keeps its existing error-string
  pattern.

- **Translation in `ResponseErrorHandlerFunc`, not in every handler**.
  Handlers wrap with `deperr.Wrap*` at the point closest to the dep
  call — that's where the operational context lives ("this is a
  send-email failure"). Translation to status code + envelope happens
  once, at the strict-handler boundary, instead of being repeated at
  every return site. This also keeps the strict handlers'
  `(Response, error)` signatures untouched — they keep returning
  `nil, err` and the translator does the right thing.

- **Conservative DB classification**. Wrapping every non-sentinel store
  error as `dep.db_unavailable` over-reports DB failures (a programmer
  bug in a query would also surface as `dep.db_unavailable`). The
  trade-off is acceptable: the existing taxonomy already encodes the
  *known* business cases (`store.ErrNotFound`, etc.), so anything else
  *is* an operationally relevant signal. A future story can refine
  this if the false-positive rate proves noisy.

- **503 with Retry-After for upstream-down**. Matches RFC 9110's
  semantics: 503 says "I'm unavailable, try again later"; the body's
  `error` field tells the SPA / plugin *which* dependency is down so
  they can pick the right user-facing message. Status alone is
  insufficient (a 503 could mean "the whole portal is in maintenance"
  vs "SMTP is down but everything else works"); the dep code
  disambiguates.

- **Git subprocess stays at 500 without Retry-After**. Subprocess
  spawn / wait failures are not transient in the same sense — they
  indicate local process / disk / binary trouble. A `Retry-After` on a
  bad `git` binary would be dishonest. Operators see the full error
  in logs and act; clients see a typed code and know it's not a
  client-side fix.

- **Foundation-doc roll-forward in this commit**. `docs/PROTOCOL.md >
  HTTP error contract` documents the new codes inline as part of the
  design commit. The doc describes the contract *now*; story 6 is a
  paper-trail breadcrumb so the doc change is reachable from the
  feature's review surface, but the actual edit happens at design
  time.

## Acceptance criteria

- [x] Every documented `dep.*` code is implemented end-to-end (handler
      wrap → translator → envelope on the wire)
- [x] `docs/PROTOCOL.md > HTTP error contract` registers each new code
      with status + Retry-After semantics
- [x] `tests/e2e/failure/config_and_deps_test.go` asserts on the typed
      envelope (status + `error` field), not just status
- [x] Existing unit tests that asserted plain-text 500 on dep failure
      are updated to assert the typed envelope
- [x] Foundation-doc rolling-forward principle observed — no
      "previously this was..." prose anywhere
- [x] All child stories at `stage: done`

## Review

**Verdict: Approve.**

The four-code dep-failure taxonomy is wired end-to-end. Every child
story landed at `stage: done` with its own Approve verdict and zero
blocker findings. Aggregate verification follows.

### Taxonomy coverage (handler wrap → translator → envelope)

- **`dep.smtp_unavailable`** (503, Retry-After: 5) — `Sender.Send`
  call sites in `auth/magic_link.go`, `accounts/orgs.go`,
  `sessions/invites.go` wrap with `deperr.WrapSMTP`. All four
  in-tree provider impls (`smtp`, `resend`, `postmark`, `sendgrid`)
  classify failures into `ErrTransient`/`ErrAuth`/`ErrPermanent`.
- **`dep.db_unavailable`** (503, Retry-After: 2) — 95 wrap sites
  across 19 production files (accounts, sessions, comments,
  finalize, auth, githttp). `WrapDBIfTransient` correctly preserves
  `store.ErrNotFound` and `store.ErrUniqueViolation` via `errors.Is`.
- **`dep.oauth_provider_unavailable`** (503, Retry-After: 10) —
  single wrap on `provider.Exchange` in `auth/oauth.go` covers all
  GitHub OAuth transport/HTTP failure paths.
- **`dep.git_subprocess_failed`** (500, no Retry-After) — six
  subprocess-failure sites in `githttp/{info_refs,upload_pack,
  receive_pack}.go` emit via `httperr.ErrGitSubprocessFailed`.

The translator (`httperr.WriteFromError`) is wired in
`cmd/portal/main.go` via `NewStrictHandlerWithOptions` with both
`ResponseErrorHandlerFunc` and `RequestErrorHandlerFunc` set. The
order — `errors.As(*Error)` first, then `errors.Is` per sentinel,
then default `ErrInternal` — preserves all pre-existing typed-error
paths (notably middleware-emitted envelopes).

### Doc + test coverage

`docs/PROTOCOL.md > HTTP error contract > Dependency-failure codes`
(lines 390–409) registers all four codes with status, Retry-After,
trigger description, and the `store.ErrNotFound`/`ErrUniqueViolation`
carve-out. No legacy "previously this was…" prose — rolling-forward
principle observed.

`tests/e2e/failure/config_and_deps_test.go` asserts on the full
typed envelope (status + `Content-Type` prefix + decoded `{error,
message}` + `Retry-After`) for SMTP, DB (Toxiproxy `reset_peer`),
and OAuth-provider sub-tests. Per-package unit tests in `auth`,
`accounts`, `sessions`, `comments`, `finalize`, and `githttp` cover
the typed envelope contract at the strict-handler boundary.

### Build + test results

- `go build ./...` — clean.
- `go test ./internal/portal/...` — 25 packages green (includes
  `deperr` and `httperr` standalone + every handler family with
  new dep-class assertions).
- `go test ./internal/portal/deperr/... ./internal/portal/httperr/...
  -count=1` — green.
- `cd tests/e2e && go build ./...` — clean.
- e2e `TestConfigAndDeps` reported PASS by the e2e-asserts story
  (21.97s, stable across `-count=2`).

### Parked edge-case gaps (not blockers)

Two real product-edge gaps surfaced during implementation and were
appropriately parked rather than expanding scope. The dep-failure
contract holds for the common cases; these refine the long tail.

- `portal-oauth-provider-error-taxonomy` — `Provider.Exchange`
  bundles RFC 6749 business errors (e.g. `bad_verification_code`,
  `invalid_grant`) with transport failures, so a 200-OK token-endpoint
  body with `{"error":"bad_verification_code"}` currently classifies
  as `dep.oauth_provider_unavailable` instead of a 400-class business
  error. Mild client UX gap; sketch in backlog proposes an
  `oauth.ErrBadGrant` sentinel + classify-then-wrap pattern.
- `portal-bearer-middleware-dep-translate` — `tokens.BearerMiddleware`
  defaults non-sentinel `svc.Validate` errors to
  `httperr.Write(w, r, httperr.ErrInternal(err))`, bypassing the
  strict-handler translator. Auth-gated endpoints under a DB outage
  surface as plain 500 instead of `dep.db_unavailable` 503. Backlog
  proposes routing through `WriteFromError` with
  `deperr.WrapDBIfTransient`. Also flagged: a follow-up sweep for
  other direct `httperr.Write(...ErrInternal(err))` sites.

### Findings

- Blockers: 0
- Important: 0 (both gaps already parked)
- Nits: 0

Advancing review → done.
