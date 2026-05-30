---
id: epic-cli-browser-session-resume
kind: epic
stage: drafting
tags: [plugin, portal, ui, security]
parent: null
depends_on: [feature-cli-jam-open-in-browser]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# CLI → browser session resume (adopt the CLI's identity in the browser)

## Brief

Today `jamsesh jam new/join --open` opens the session's portal URL but the
browser cannot adopt the identity the CLI already holds. For a **playground**
session the browser lands on the join page and mints a *second* anonymous
participant (it does not resume the CLI's anonymous identity); for a **durable**
session the browser hits an unauthenticated session view and must log into the
portal separately. This epic adds a secure CLI→browser **session-resume
handoff**: a human at the terminal can open the browser and resume *their own*
CLI session as themselves — the "durable-style resume" affordance flagged when
`feature-cli-jam-open-in-browser` shipped.

This is the **reverse** of a normal CLI login (where the browser authenticates a
human and the CLI receives a token). Here the CLI already has authority and the
browser needs to adopt it — so the mechanism is the CLI→browser equivalent of an
OAuth authorization code: the CLI asks the portal to mint a scoped, single-use,
short-TTL **resume token**, opens the SPA to a resume route, and the SPA
exchanges that token for a browser-scoped session bearer. One protocol covers
both session kinds; the *issued credential* differs (anonymous bearer for
playground; a browser-scoped session token for durable — never the CLI's
long-lived OAuth refresh token).

Builds on the shipped `--open` flag (`feature-cli-jam-open-in-browser`).

## Strategic decisions

(Locked at scope time. The mechanism was chosen after a cross-model design
consult with Codex — see `## Other agent consult`.)

- **Scope / audience**: BOTH playground and durable sessions (a general
  CLI→browser identity handoff), covering `new` and `join`. — User-selected;
  makes this an epic, not a single feature.
- **Mechanism**: portal-minted **single-use, short-TTL resume token carried in
  the URL fragment**; the SPA exchanges it for a browser bearer. NOT a
  localhost-loopback callback (loopback is for browser→CLI, the wrong direction
  here) and NOT the raw bearer in the URL. — Confirmed standard-ish + reasonably
  secure by the Codex consult; matches the user's lean.
- **One mechanism, two credential policies**: the resume-token endpoint shape is
  identical for both; the *exchange* returns the anonymous session bearer for
  playground, and a freshly-minted browser-scoped session token for durable
  (treat the CLI bearer as proof the user is authenticated; enter the same
  post-login state the SPA's OAuth flow produces — do NOT expose the CLI's
  refresh token to the SPA).
- **Security bar**: reasonably secure, not bulletproof (playground especially is
  ephemeral, anonymous, low-value). Standard safeguards below are required;
  bearer rotation / advanced hardening is out of scope for v1.

## Design direction (from the Codex consult — input for epic-design)

Protocol (one shape, both kinds):
- `POST /api/session-resumes` — authenticated with the CLI bearer (anonymous or
  durable). Returns `{ resume_token, expires_in: 30–120s, session_id }`. Server
  stores only a **hash** of the token, single-use, bound to `session_id` and to
  the minting account/identity.
- CLI opens the browser to a resume route with the token in the **fragment**:
  - playground: `/playground/s/{session_id}/resume#rt=<token>`
  - durable:    `/orgs/{orgId}/sessions/{session_id}/resume#rt=<token>` (route
    shape TBD in design; mirror the existing `session-view` path family)
- SPA resume route: (1) read `location.hash`; (2) immediately
  `history.replaceState(...)` to strip the fragment; (3)
  `POST /api/session-resumes/exchange` with the token; (4) portal validates
  single-use + TTL + session binding + account binding and returns the
  appropriate browser bearer; (5) SPA stores it (playground → the in-memory
  `_playgroundContext` rune via `auth.setPlaygroundContext`; durable → the same
  post-OAuth-login state the SPA already uses); (6) land in the live session AS
  the CLI identity.

Required safeguards (carry into the relevant child features):
- TTL 30–120s; single-use with atomic consume; bind to `session_id` + minting
  account; store only a token hash server-side; return the minimal browser
  credential.
- Never log the resume token, the full URL, the fragment, or the exchange body.
- SPA strips the fragment before loading third-party assets / further nav; set
  `Referrer-Policy: no-referrer` (or `same-origin`); exchange is same-origin
  only; rate-limit; generic failure messages.
- Durable: do NOT hand the CLI refresh token to the SPA — mint a browser-scoped
  session for the same account instead.

CLI surface (design to settle the exact shape): either extend `--open` to
resume-when-possible, or add an explicit `jamsesh open --resume` / `jamsesh
resume`. Reuse the `cmd/jamsesh/internal/osopen` opener and the per-session
bearer in CLI state.

## Other agent consult

Cross-model design consult (Codex, 2026-05-30, single advisory pass at user
request — "what's standard for this, reasonably secure not bulletproof").
Surveyed `gh`, `aws sso`, `gcloud`, `stripe`, `vercel`, `netlify`, `tailscale`,
`databricks`, `supabase`, RFC 8252, MDN URL-fragment semantics.

Accepted (drives the decisions above):
- Recommended Option A (server-minted one-time resume token in the fragment) as
  the clean CLI→browser analog of an OAuth auth code; ranked loopback as the
  wrong direction for this problem.
- One protocol, different issued credential for playground vs durable; never
  expose the durable refresh token to the SPA.
- The full safeguard list (TTL, single-use, hash-at-rest, session/account
  binding, fragment-not-query, `replaceState`, no-referrer, same-origin
  exchange, no logging, rate limits).

Note on fragment safety (accepted with eyes open): fragments aren't sent to the
server or in access logs, but remain visible to extensions, history, crash
restore, copy-paste, screenshots — hence the value must be a transient
single-use *code*, never the real credential.

## Likely feature decomposition (for epic-design — not yet binding)

- **Portal resume-token contract** (`portal`, `security`): the two endpoints,
  hashed single-use token store with TTL + session/account binding, dual
  credential issuance (anonymous vs browser-scoped durable). Owns the threat
  model. Likely the first feature (others depend on the contract).
- **CLI resume handoff** (`plugin`): mint the resume token, open the browser to
  the resume route (reuse `osopen`); CLI surface decision (`--resume` vs
  command). Depends on the portal contract.
- **SPA resume route** (`ui`): the `/…/resume` route(s), fragment read +
  `replaceState`, exchange call, store into the correct auth state, error/expiry
  UX. Depends on the portal contract.

## Foundation-doc impact (deferred to implementation — per the rolling-foundation present-tense rule)

This epic introduces an unbuilt flow, so foundation docs are NOT pre-written at
scope time (the project rule: docs describe the system as it is NOW; the item
body is the durable direction). When the child features land, roll forward:
- `docs/SECURITY.md` — the resume-token flow + threat model (alongside the
  anonymous-bearer and OAuth sections).
- `docs/openapi.yaml` / `docs/SPEC.md` — the two new endpoints + types.
- `docs/ARCHITECTURE.md` — the CLI→browser handoff component/flow.
- `docs/UX.md` — the resume flow in the create/join journeys.

## Out of scope

- Bearer rotation and advanced anti-hijack hardening (noted as a future pass in
  SECURITY.md; not a v1 requirement).
- Changing the existing OAuth browser-login flow (`auth/browser.go`) — the
  resume handoff is additive, in the opposite direction.
- A general "share a live link that logs anyone in" — resume is for the
  identity the CLI already holds, bound to the minting account.
