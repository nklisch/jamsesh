---
id: epic-cli-browser-session-resume-portal-contract-exchange-credential
kind: story
stage: done
tags: [portal, security]
parent: epic-cli-browser-session-resume-portal-contract
depends_on: [epic-cli-browser-session-resume-portal-contract-endpoints-mint]
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
---

# Resume exchange endpoint + dual credential issuance ⚠

Implements **Unit 3** (the trickiest unit) of
`epic-cli-browser-session-resume-portal-contract`. See the feature body + its
`## Risks`.

## Scope

- `internal/portal/sessionresume/`: implement `POST /api/session-resumes/exchange`
  in the PUBLIC route group with its own rate limit — UNAUTHENTICATED (the resume
  token is the sole credential; ignore any ambient `Authorization` header). Use
  the **winner-returning** `ConsumeResumeToken` (validate-not-used + not-expired +
  consume in ONE atomic statement): a returned row is the single-use winner;
  zero rows → GENERIC failure (no oracle), NO credential issued. Don't do a
  separate Get-then-Consume (TOCTOU).
- Branch on the bound account's `is_anonymous`:
  - durable → `tokens.IssueShortLived(accountID, tokens.AccessTokenTTL)`
    (access-only, no refresh in the response).
  - playground → NEW `tokens.Service` method
    `IssueAnonymousSessionBearerForExistingAccount(ctx, accountID, sessionID,
    ttl)` — issues a session-scoped bearer for the EXISTING anonymous account
    (no new `accounts` row), shape-identical to the original anon bearer
    (`kind=anonymous_session_bearer`, `session_id` FK), TTL = remaining time to
    the session hard-cap. Preconditions (fail-fast): `account.is_anonymous`;
    account already a member of the target session; session active w/ positive
    TTL; durable (non-anonymous) account rejected.
- Return `{ bearer, expires_at, session_id, org_id, kind, account_id,
  display_name }` — identity metadata lets the SPA confirm an account-mismatch
  switch without a second `/me` probe.

## Pre-work (do FIRST — see feature Risks)

Read the playground bearer-validation / authorization path
(`internal/portal/tokens` Validate + the playground handler authz) to confirm
the new method produces a token existing authz accepts as a session member
BEFORE finalizing the method shape.

## Acceptance criteria

- [x] Expired / already-used / unknown token → generic failure (no oracle
      distinguishing them); single-use enforced under CONCURRENT exchange — two
      parallel exchanges of one token yield exactly ONE bearer (only the
      `RETURNING` winner issues).
- [x] Mounted in the public route group + own rate limit; ambient
      `Authorization` header is ignored (exchange is unauthenticated).
- [x] The new tokens method rejects a non-anonymous account and a
      non-member / ended-session; exchange response includes `account_id` +
      `display_name` identity metadata.
- [x] **Playground exchange creates NO new `accounts` row** — same `account_id`
      as the minting bearer; the issued bearer Validates as a session member.
- [x] Durable exchange returns an `IssueShortLived` access token; NO refresh
      token in the response.
- [x] Raw token never logged (mint response, exchange body, errors).
- [x] `go build ./...`, `go vet`, handler + tokens tests pass.

## Implementation notes

### Pre-work findings

- `IssueAnonymousSessionBearer` creates a fresh anon account row + a bearer
  (`CreateAnonymousBearer`) in a single transaction. The new method
  `IssueAnonymousSessionBearerForExistingAccount` skips the account-create
  step and calls `CreateAnonymousBearer` directly with the existing accountID.
  The row shape is identical (`kind=anonymous_session_bearer`, `session_id` FK).

- `handlerauth.RequireSessionMember` (and `RequireAnonymousSessionMember`)
  calls `GetSessionMember(orgID, sessionID, accountID)`. A bearer created via
  `CreateAnonymousBearer` for the existing anon account + sessionID is accepted
  as a session member on the SAME session as long as the account row in
  `session_members` is present. The test
  `TestExchangeSessionResume_Playground_NoNewAccount` confirms this path with
  `handlerauth.RequireSessionMember`.

- No new account row is created on exchange — the existing anon account (from
  the original playground create/join) receives a fresh bearer scoped to the
  same session.

### Files changed

- `docs/openapi.yaml` — `SessionResumeExchangeRequest`, `SessionResumeExchangeResponse`
  schemas; `POST /api/session-resumes/exchange` path with `operationId: exchangeSessionResume`.
- `internal/api/openapi/server.gen.go` — regenerated (make generate-api-go).
- `internal/portal/tokens/service.go` — `IssueAnonymousSessionBearerForExistingAccount`
  added to `Service` interface.
- `internal/portal/tokens/service_impl.go` — implementation; rejects non-anonymous
  accounts with `ErrForbidden`.
- `internal/portal/sessionresume/handler.go` — extended `sessionResumeStore`
  to include `store.AccountStore`.
- `internal/portal/sessionresume/exchange.go` — `ExchangeSessionResume` handler;
  playground + durable credential branches; no-oracle generic failure.
- `internal/portal/sessionresume/exchange_test.go` — security-critical test suite.
- `cmd/portal/main.go` — `ExchangeSessionResume` delegation + public route
  `/session-resumes/exchange` with own rate limiter.
- All shim test files updated with `ExchangeSessionResume panic("not wired")`.
- `internal/portal/tokens/middleware_test.go` — `mockService` updated.
- `internal/portal/playground/handler_test.go` — `failingTokensService` updated.

### Security decisions

- Exchange is unauthenticated: only the resume token matters; ambient context
  account is never read by `ExchangeSessionResume`.
- All three failure cases (unknown, expired, already-used) return the same
  `auth.invalid_token` shape — no oracle.
- Raw token hash is computed inline; raw value is discarded before any logging.
- Durable path: `IssueShortLived` produces `kind=access` — no refresh token.
- Playground path: TTL bound to session `HardCapAt`; zero/negative TTL returns
  generic failure (session already ended).
