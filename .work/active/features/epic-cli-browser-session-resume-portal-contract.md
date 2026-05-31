---
id: epic-cli-browser-session-resume-portal-contract
kind: feature
stage: done
tags: [portal, security]
parent: epic-cli-browser-session-resume
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Portal resume-token contract

## Brief

The foundational, server-side half of the CLI→browser session-resume handoff:
the protocol both clients (CLI, SPA) build against. Exposes two endpoints — a
**mint** call authenticated with the CLI's existing session bearer that issues a
single-use, short-TTL resume token, and an **exchange** call that trades a valid
resume token for a browser-scoped session bearer. Owns the token store and the
threat model for the whole epic.

It must support **both** session kinds with one protocol shape but two credential
policies (per the epic's locked Strategic decisions): exchanging for a
**playground** session returns the anonymous session bearer; exchanging for a
**durable** session mints a fresh **browser-scoped** session token for the same
account — never the CLI's long-lived OAuth refresh token.

Does NOT cover the CLI surface (sibling `…-cli-handoff`) or the browser
consume route (sibling `…-spa-route`). This feature is the contract they share,
so it lands first.

## Epic context

- Parent epic: `epic-cli-browser-session-resume`
- Position in epic: **foundation feature** — both `…-cli-handoff` and
  `…-spa-route` depend on its endpoints + types. On the critical path.

## Foundation references

- `internal/portal/tokens/` — `IssueAnonymousSessionBearer(ctx, sessID,
  nickname, ttl)` is the playground credential-issuance precedent; the durable
  browser-scoped token issuance is new here.
- `db/queries/{sqlite,postgres}/magic_link_tokens.sql` + the
  `magic_link_tokens` schema — precedent for a **hashed, single-use, TTL**
  token store; the resume-token store should mirror this shape (dual-dialect per
  the `dual-dialect-mirror-queries` pattern; store only a token hash).
- `internal/portal/playground/handler.go` and `internal/portal/sessions/` —
  where session ownership / membership checks live; the mint endpoint authorizes
  against the CLI bearer's session binding.
- `internal/portal/ratelimit/` — exchange + mint endpoints need rate limiting.
- `docs/openapi.yaml` — the two endpoints + request/response types are net-new
  (no existing `resume` surface); contract types are generated, per the
  Generated-Contracts principle.

## Locked direction (inherited from the epic — do not re-litigate)

See the parent epic's `## Strategic decisions` and `## Design direction`. Key
points this feature owns:
- `POST /api/session-resumes` (auth: CLI bearer) → `{ resume_token,
  expires_in: 30–120s, session_id }`; store only a hash; single-use; bound to
  `session_id` + minting account.
- `POST /api/session-resumes/exchange` (the token) → the appropriate browser
  bearer (anonymous for playground; browser-scoped session token for durable).
- Safeguards owned here: TTL, atomic single-use consume, hash-at-rest,
  session/account binding, never log the token, same-origin exchange, rate
  limits, generic failure messages, do NOT expose the durable refresh token.

The feature-design pass settles: exact package placement (new
`internal/portal/sessionresume` vs extend `sessions`/`playground`), the durable
browser-scoped credential mechanism (new token `kind` vs reuse of an existing
issuance path), schema, and per-endpoint tests.

## Decomposition-review findings (Codex, accepted — fold into this feature's design)

- **Playground exchange must NOT mint a new participant.** The goal is to resume
  the CLI's *existing* anonymous identity. `tokens.IssueAnonymousSessionBearer`
  creates a *fresh* anonymous account + member row — calling it at exchange time
  would defeat the purpose. This feature must issue a browser bearer for the
  **existing** anonymous account/session bound to the resume token (a new
  issuance path that reuses the account the token was minted under). [BLOCKER]
- **Durable mint authorization must be explicit.** Durable CLI bearers are
  *account-scoped* OAuth tokens, not session-bound (per
  `cmd/jamsesh/state/state.go` per-session fallback). The mint request must carry
  `org_id` + `session_id` and do a membership check — mirror
  `internal/portal/finalize/fetch_token.go` — rather than assume a session-bound
  durable bearer.
- **The resume route path + fragment key (`rt`) are part of THIS contract.**
  Define them here (single source of truth) so both consumers
  (`…-cli-handoff` builds the URL, `…-spa-route` registers the route) reference
  one definition and can be designed independently without drifting. This
  resolves the launch-URL coupling without adding a CLI→SPA dependency edge.
- **Durable browser-scoped credential is a distinct design unit (likely its own
  story).** Settle: response schema, token kind + TTL, whether the SPA receives
  an access-only browser session vs the existing `setTokens(access, refresh)`
  pair (do NOT hand the refresh token to the SPA), revocation semantics, and how
  it enters the SPA's post-login state.
- **Exchange must be safe against ambient browser auth (server side).** A browser
  already logged in as another account may send an unrelated `Authorization`
  header to the public exchange. Define server behavior: treat exchange as
  unauthenticated (the resume token is the sole credential) and reject/ignore a
  mismatched ambient bearer. (Client side handled in `…-spa-route`.)

## Architectural choice

Mirror the **magic-link token infrastructure**, which already solves "hashed,
single-use, TTL token + exchange for a credential": a `resume_tokens` table
shaped like `magic_link_tokens`, a mint endpoint shaped like
`finalize.IssueFetchToken` (account-from-context + `checkSessionMembership`),
and an exchange endpoint shaped like the magic-link exchange. Credentials reuse
the existing `tokens.Service`: durable → `IssueShortLived` (the exact
fetch-token mechanism — a short-TTL `oauth_tokens` access row, no refresh);
playground → a new shape-preserving method that issues a session-scoped bearer
for the **existing** anonymous account.

Rejected: a bespoke token subsystem (re-solves what magic-link already does);
putting the durable credential behind a new `oauth_tokens.kind` (the bearer
middleware already honours per-row `expires_at`, so `IssueShortLived` needs no
new kind — confirmed by fetch-token).

The **mint response returns the fully-formed `resume_url`** (fragment embedded),
so the CLI opens it verbatim and never constructs the route — this makes the
portal the single source of truth for the route shape + `rt` fragment key and
eliminates CLI/SPA drift (resolves the Codex launch-route finding).

## Design decisions

(No Phase-4.5 user checkpoint — scope/mechanism/credential-policy/safeguards were
locked in the epic; the codebase precedents pin the rest. Knobs decided here:)
- **Resume route shape (canonical)**: playground →
  `/playground/s/{sessionId}/resume#rt=<token>`; durable →
  `/orgs/{orgId}/sessions/{sessionId}/resume#rt=<token>`. Fragment key `rt`.
  Returned pre-built as `resume_url` by mint.
- **Resume-token TTL**: 60s (mid 30–120s range).
- **Durable browser credential**: `IssueShortLived(accountID, AccessTokenTTL=1h)`
  — access-only, no refresh to the SPA; SPA re-resumes / re-logs-in on expiry.
- **Playground browser credential**: a session-scoped bearer for the existing
  anonymous account, TTL = remaining time to the session hard-cap (mirrors the
  original anonymous bearer). Requires a new `tokens.Service` method (Unit 3).
- **Exchange is unauthenticated**: the resume token is the sole credential; the
  handler ignores any ambient `Authorization` header (no account-from-context).
- **Mint authorization**: `tokens.AccountFromContext` + `checkSessionMembership`
  (mirror finalize) — works for durable (account-scoped bearer) and playground
  (anonymous account is a session member); body carries `org_id`+`session_id`.

## Implementation Units

### Unit 1: `resume_tokens` dual-dialect store
**Story**: `epic-cli-browser-session-resume-portal-contract-token-store`
**Files**: `db/schema/{sqlite,postgres}.sql`,
`db/queries/{sqlite,postgres}/resume_tokens.sql`, regenerated sqlc, store
adapter methods in `internal/db/store/`.

```sql
CREATE TABLE resume_tokens (
    id          TEXT PRIMARY KEY,
    token_hash  TEXT NOT NULL UNIQUE,
    session_id  TEXT NOT NULL,
    org_id      TEXT NOT NULL,
    account_id  TEXT NOT NULL,   -- minting account (durable user OR existing anon account)
    issued_at   DATETIME NOT NULL,
    expires_at  DATETIME NOT NULL,
    used_at     DATETIME
);
```
Queries (dual-dialect per `dual-dialect-mirror-queries`): `CreateResumeToken
:one`, `GetResumeTokenByHash :one`, and a **winner-returning** consume —
`ConsumeResumeToken :one` doing `UPDATE resume_tokens SET used_at=? WHERE
token_hash=? AND used_at IS NULL AND expires_at > ? RETURNING *`. ⚠ Do NOT copy
`magic_link_tokens`'s `ConsumeResumeToken :exec` — the generated `:exec` discards
rows-affected, so two concurrent exchanges could both read-unused, both consume
with nil error, and both issue credentials. The exchange handler treats
"`RETURNING` produced a row" as the single-use **winner** signal; a zero-row
result (already used / expired) issues NO credential. (The combined
validate+expiry+consume in one statement also removes a TOCTOU between the
GetByHash check and the consume.)

**Acceptance**: dual-dialect parity (identical names/columns/org-scoping); sqlc
generates clean; adapter methods covered; `ConsumeResumeToken` is atomic
single-use (second consume affects 0 rows).

### Unit 2: contract + mint endpoint
**Story**: `epic-cli-browser-session-resume-portal-contract-endpoints-mint`
**Files**: `docs/openapi.yaml` (BOTH ops + types — generated), the resume
handler package (new `internal/portal/sessionresume/` — keeps the surface
distinct), `cmd/portal` wiring, `internal/portal/ratelimit` wiring.

`POST /api/session-resumes` (auth: CLI bearer, under bearer middleware) — body
`{ org_id, session_id }`; `AccountFromContext` + `checkSessionMembership`
(401/403/404 like fetch-token); generate a random token, store `sha256` hash +
binding, return **only** `{ resume_url, expires_in, session_id }`. The raw token
appears ONCE, inside `resume_url`'s fragment — do NOT also return a separate
`resume_token` field (redundant secret surface + drift risk now that `resume_url`
is the single source of truth). Rate-limited.

**Acceptance**: mounted under bearer middleware; non-member → 403; unknown
session → 404; no bearer → 401; success stores a hashed (never raw) token bound
to account+session and returns a `resume_url` with the `rt` fragment + the
canonical path; response has no standalone token field; token never logged.

### Unit 3: exchange endpoint + dual credential issuance  ⚠ trickiest
**Story**: `epic-cli-browser-session-resume-portal-contract-exchange-credential`
**Files**: the resume handler (exchange op), `internal/portal/tokens/` (new
method), exchange tests.

`POST /api/session-resumes/exchange` — mounted in the PUBLIC route group, its
own rate limit, UNAUTHENTICATED: the resume token is the sole credential and any
ambient `Authorization` header is ignored. Body `{ resume_token }`; the
**winner-returning** `ConsumeResumeToken` (Unit 1) does lookup-by-hash +
not-used + not-expired + consume in ONE atomic statement — a returned row is the
single-use winner; a zero-row result → generic failure, NO credential issued (no
oracle distinguishing missing/expired/used). Then branch on the consumed row's
bound account `is_anonymous`:
- durable → `tokens.IssueShortLived(accountID, AccessTokenTTL)`;
- playground → new method:
```go
// IssueAnonymousSessionBearerForExistingAccount mints a session-scoped bearer
// for an EXISTING anonymous account (no new account row), pinned to sessionID,
// expiring at the session hard-cap. Mirrors IssueAnonymousSessionBearer minus
// the account-creation step, so the issued token is shape-identical to the
// CLI's original anonymous bearer (kind=anonymous_session_bearer, session_id FK).
// Preconditions (fail-fast): account.is_anonymous MUST be true; the account MUST
// already be a member of sessionID; the session MUST be active with positive
// remaining TTL. A durable (non-anonymous) account is rejected here.
IssueAnonymousSessionBearerForExistingAccount(ctx, accountID, sessionID string, ttl time.Duration) (rawToken string, expiresAt time.Time, err error)
```
Return `{ bearer, expires_at, session_id, org_id, kind: playground|durable,
account_id, display_name }` — the identity metadata (account_id + display) lets
the SPA detect a mismatch with an already-logged-in account and run its
confirm-switch (spa-route Design decisions) WITHOUT a second `/me` probe.

**Acceptance**: mounted public + rate-limited + ignores ambient `Authorization`;
expired/used/unknown token → generic failure (no oracle); single-use enforced
under CONCURRENT exchange (only the `RETURNING`-winner issues a credential — two
parallel exchanges of one token yield exactly one bearer); durable exchange
returns an `IssueShortLived` access token (no refresh in the response);
playground exchange returns a bearer for the SAME anonymous account+session (NO
new `accounts` row — assert account count unchanged) that Validate accepts as a
session member; the new tokens method rejects a non-anonymous account and a
non-member/ended-session.

## Implementation Order

1. Unit 1 (`…-token-store`) — depends on: `[]`
2. Unit 2 (`…-endpoints-mint`) — depends on: `[…-token-store]` (owns the openapi
   contract for both ops + the mint handler)
3. Unit 3 (`…-exchange-credential`) — depends on: `[…-endpoints-mint]` (needs the
   generated exchange-op types + the store; isolates the trickiest credential
   logic + the new tokens method)

Mostly sequential (shared handler package + generated openapi types). The
epic-level parallelism is CLI ∥ SPA after this whole feature lands.

## Testing

- Unit 1: dual-dialect store tests (both sqlite + postgres adapters); atomic
  single-use consume; round-trip hash storage. Reuse the `testenv-harness-struct`
  + `dual-dialect-mirror-queries` patterns.
- Unit 2: handler tests via the `strict-server-partial-handler-shim` pattern;
  membership 401/403/404 matrix (mirror `finalize` fetch-token tests); assert the
  stored token is hashed and the response `resume_url` shape; assert the token is
  absent from logs.
- Unit 3: the security-critical surface — expired/used/unknown → generic error;
  ambient-auth-ignored; **playground exchange creates NO new account** (assert
  `accounts` row count unchanged, same account_id as the minting bearer);
  durable exchange returns access-only; single-use enforced under concurrent
  exchange (atomic consume).

## Risks

- **Playground existing-account bearer (Unit 3)** is the riskiest: the new
  tokens method must produce a token shape-identical to the original anonymous
  bearer (kind + session_id FK) so existing playground authz accepts it. The
  implementing story must verify playground authorization actually keys off the
  account's session membership (and/or the token's session_id FK) before
  finalizing the method — read the playground bearer-validation path first.
- **Token-in-logs**: the raw token must never hit logs (mint response, exchange
  body, error paths). Enforced in handler tests.
- Foundation-doc roll-forward (deferred): `docs/openapi.yaml` (+ generated
  types), `docs/SECURITY.md` (resume-token flow + threat model),
  `docs/ARCHITECTURE.md` (CLI→browser handoff — assigned to this feature),
  `docs/SPEC.md` if a new constraint emerges. Written when the endpoints land.

## Final-review findings (Codex, accepted — folded above)

Final cross-model design review (Codex, 2026-05-30). **Verdict was Block** on a
real consume-race; fixed in the design before any code:
- [BLOCKER, fixed] `ConsumeResumeToken` is now a winner-returning `:one`
  (`UPDATE … WHERE token_hash=? AND used_at IS NULL AND expires_at > ?
  RETURNING *`) — the generated `:exec` precedent discards rows-affected and
  would let concurrent exchanges double-issue. Only the `RETURNING`-winner
  issues a credential. (Units 1 + 3.)
- [important, fixed] Mint returns only `resume_url` (+ `expires_in`,
  `session_id`) — dropped the redundant `resume_token` field. (Unit 2.)
- [important, fixed] Exchange response carries identity metadata
  (`account_id`, `display_name`) so the SPA can confirm an account-mismatch
  switch without a `/me` probe. (Unit 3 → consumed by spa-route.)
- [important, fixed] New anon-existing-account tokens method has explicit
  fail-fast preconditions (is_anonymous, existing session membership, active
  positive-TTL session; durable rejected). (Unit 3.)
- [important, fixed] Route mounting is now AC: mint under bearer middleware;
  exchange in the public group + own rate limit + ignores ambient auth.
  (Units 2 + 3.)

Confirmed safe (no change needed): CSRF/open-redirect are not primary risks
(bearer headers not cookies; same-origin exchange; resume routes carry no
external return target); `Referrer-Policy: no-referrer` is already global
(`internal/portal/router/security_headers.go`); playground authz checks session
membership after token validation (`internal/portal/handlerauth/handlerauth.go`).

**Confirm pass (Codex, 2026-05-30): clean.** Blocker resolved by the
winner-returning consume; no new blocker/important/nit from the fixes;
dual-dialect `UPDATE … RETURNING` confirmed already used in-repo, so the
approach is implementable. Design is ready for implementation.

## Implementation (orchestrator run, 2026-05-30)

All 3 stories implemented (3 sequential single-item waves — linear chain, shared
`sessionresume` package + generated artifacts) and advanced to `review`:

1. `…-token-store` (commit `fa43e5f`) — `resume_tokens` dual-dialect store
   (sqlc + goose migrations sqlite 00019 / postgres 00020); **winner-returning**
   `ConsumeResumeToken :one … RETURNING *` (validate+expiry+consume atomic, no
   TOCTOU); store-only-hash; 7 store tests incl. single-use + expired.
2. `…-endpoints-mint` (commit `0aabd5b`) — new `internal/portal/sessionresume/`
   package; `POST /api/session-resumes` (bearer group, rate-limited 10/min);
   `AccountFromContext` + replicated `checkSessionMembership` (401/403/404);
   returns only `{ resume_url, expires_in:60, session_id }` (raw token only in
   the `#rt=` fragment); 7 handler tests.
3. `…-exchange-credential` (commit `569afd8`) — `POST /api/session-resumes/exchange`
   (PUBLIC group, own rate limit, unauthenticated); winner-returning consume →
   generic no-oracle failure; durable → `IssueShortLived` (no refresh),
   playground → new `tokens.IssueAnonymousSessionBearerForExistingAccount`
   (existing anon account, shape-identical bearer, rejects non-anonymous);
   identity metadata in response; 15 tests incl. no-new-account + no-oracle +
   single-use + ambient-auth-ignored.

Verification: `go build ./...`, `go vet ./cmd/... ./internal/...`, and
`go test ./internal/portal/sessionresume/… ./internal/portal/tokens/…
./internal/db/…` all pass. Deviation noted for review: the single-use test is
sequential back-to-back (atomicity is in the SQL `UPDATE … WHERE used_at IS NULL`,
not goroutine scheduling) rather than true-parallel goroutines.

## Milestone review (Codex xhigh, 2026-05-30) — Block, then fixed

Major-milestone cross-model review. Verdict **Block** on a real playground-mint
defect; all findings accepted and fixed before advancing to done:

- [BLOCKER] **Playground mint 403'd.** `membership.go` required `GetOrgMember`
  before session membership, but playground participants are anonymous SESSION
  members (never in the playground org's `org_members`). The mint/exchange tests
  masked it by adding playground org-membership in setup. Fix: membership check
  accepts a session member without org membership for the playground org (mirror
  `handlerauth.RequireAnonymousSessionMember`); corrected the tests to NOT add
  org-membership for anon accounts so they exercise the real path.
- [important] Sequential "concurrency" test → real concurrent single-use test.
- [important] `ConsumeResumeTokenParams.UsedAt` nullable at the store boundary →
  hardened so consume always marks `used_at` non-null.
- [important] No-new-account test counted org-members not account rows →
  strengthened to assert the `accounts` row count is unchanged.
- [important] `sessionresume` not wired to the portal test clock → use
  `NewWithClock` in `cmd/portal/main.go` (mirror finalize).
- [important] Consumed-token→deleted-account returned a wrapped DB error not the
  generic failure → map account-not-found to the generic 401; add FK/cascade on
  `resume_tokens` (account/session deletion cleans up tokens).
- [good] single-use path airtight; exchange public + ambient-auth-ignored;
  durable no-refresh + identity metadata; hash-only persistence.

Fix landed (commit `2bf88ee`). Codex xhigh **confirm pass**: blocker resolved
(playground mint/exchange work session-only; durable unchanged; concurrency test
real + `-race`-clean; `UsedAt` hardened; clock wired) — surfaced ONE new
important issue: the FK migrations could fail on pre-existing **orphan**
`resume_tokens`. Fixed (commit after): sqlite recreate copies only rows with a
live account+session; postgres deletes orphans before `ADD CONSTRAINT`.
`internal/db` migration suite + sessionresume `-race` green. **Feature → done.**
