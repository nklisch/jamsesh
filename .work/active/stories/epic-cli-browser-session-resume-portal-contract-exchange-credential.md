---
id: epic-cli-browser-session-resume-portal-contract-exchange-credential
kind: story
stage: implementing
tags: [portal, security]
parent: epic-cli-browser-session-resume-portal-contract
depends_on: [epic-cli-browser-session-resume-portal-contract-endpoints-mint]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
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

- [ ] Expired / already-used / unknown token → generic failure (no oracle
      distinguishing them); single-use enforced under CONCURRENT exchange — two
      parallel exchanges of one token yield exactly ONE bearer (only the
      `RETURNING` winner issues).
- [ ] Mounted in the public route group + own rate limit; ambient
      `Authorization` header is ignored (exchange is unauthenticated).
- [ ] The new tokens method rejects a non-anonymous account and a
      non-member / ended-session; exchange response includes `account_id` +
      `display_name` identity metadata.
- [ ] **Playground exchange creates NO new `accounts` row** — same `account_id`
      as the minting bearer; the issued bearer Validates as a session member.
- [ ] Durable exchange returns an `IssueShortLived` access token; NO refresh
      token in the response.
- [ ] Raw token never logged (mint response, exchange body, errors).
- [ ] `go build ./...`, `go vet`, handler + tokens tests pass.
