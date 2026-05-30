---
id: epic-cli-browser-session-resume-portal-contract
kind: feature
stage: drafting
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

## Foundation-doc roll-forward (at implementation, per present-tense rule)

`docs/openapi.yaml` (+ generated types), `docs/SECURITY.md` (resume-token flow +
threat model), `docs/ARCHITECTURE.md` (the CLI→browser handoff component/flow —
assigned to this feature), `docs/SPEC.md` if a new constraint emerges. Not
written until the endpoints exist.
