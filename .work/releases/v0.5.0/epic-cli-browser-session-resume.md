---
id: epic-cli-browser-session-resume
kind: epic
stage: done
tags: [plugin, portal, ui, security]
parent: null
depends_on: [feature-cli-jam-open-in-browser]
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
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

## Decomposition

Split by **independently-deliverable component with a real producer→consumer
dependency**, not by layer-slice: the portal contract is the shared protocol
both clients build against, and the CLI and SPA are two *independent* consumers
(different languages/areas, no shared code, each independently shippable and
testable). This gives autopilot genuine parallelism — CLI ∥ SPA once the
contract lands — and avoids straddling every component across a
playground/durable split (the playground-vs-durable difference is just a
credential-policy branch inside each component, handled by the contract at
exchange time). The work closely parallels the existing magic-link
infrastructure (`magic_link_tokens` store, `MagicLinkExchange.svelte`,
`IssueAnonymousSessionBearer`), which de-risks all three.

### Child features

- `epic-cli-browser-session-resume-portal-contract` — the two endpoints
  (mint + exchange), hashed single-use TTL token store, dual credential
  issuance, threat model — depends on: `[]` (foundation)
- `epic-cli-browser-session-resume-cli-handoff` — CLI mints the token + opens
  the browser to the resume route (reuses `osopen`) — depends on:
  `[epic-cli-browser-session-resume-portal-contract]`
- `epic-cli-browser-session-resume-spa-route` — `/…/resume` route: fragment
  read + `replaceState` + exchange + store-into-auth-state + error/expiry UX —
  depends on: `[epic-cli-browser-session-resume-portal-contract]`

### UI alignment

No net-new mockup at the epic tier: the only UI surface (the SPA resume route)
is a transitional exchange screen that reuses the existing `MagicLinkExchange` /
`JoinerOutcome` patterns + the established design system. Noted on the SPA
feature; `feature-design` may fall back to `/ux-ui-design:screens` only if the
error/expiry UX proves novel.

### Design questions

No epic-level strategic ambiguities remained at decomposition time — scope,
mechanism, credential policy, and safeguards were all locked in `## Strategic
decisions` (user scope answers + Codex consult). Remaining choices are
feature-level (CLI surface; durable browser-credential mechanism; resume route
path/shape) and are deferred to the per-feature design passes.

### Decomposition risks

- **Durable browser-scoped credential issuance** (in `…-portal-contract`) is the
  riskiest unit: minting a browser session from a CLI OAuth bearer must enter
  the SPA's existing post-login state without exposing the refresh token, and
  must interact cleanly with the existing OAuth/token model. Flag for careful
  design + a focused threat-model pass in that feature.
- Critical path is contract → (CLI ∥ SPA); inherent and acceptable (clients
  can't precede their contract). The CLI∥SPA parallelism is preserved.

## Other agent review (decomposition, Codex, 2026-05-30)

Cross-model peer review of the decomposition (single advisory pass).
**Verdict: approve the 3-feature cut, with changes** — the shape is right (not a
layer-slice anti-pattern; CLI and SPA are independent consumers of the
contract). All findings accepted and folded into the feature bodies under each
feature's `## Decomposition-review findings`:

- [blocker] CLI must not leak the resume token to terminal scrollback (the
  existing `openInBrowser` prints the full URL) → `…-cli-handoff`.
- [blocker] Playground exchange must issue a bearer for the **existing**
  anonymous account, not mint a new participant via
  `IssueAnonymousSessionBearer` → `…-portal-contract`.
- [important] Launch route path + `rt` fragment key are part of the **contract**
  (single source of truth) — resolves the CLI/SPA launch coupling without a new
  dependency edge → `…-portal-contract` (referenced by both consumers).
- [important] Durable browser-scoped credential = a distinct design unit/story
  (schema, token kind/TTL, no refresh-token-to-SPA, post-login state) →
  `…-portal-contract`.
- [important] Durable mint must carry org/session + membership check (durable
  CLI bearers are account-scoped) → `…-portal-contract` + `…-cli-handoff`.
- [important] Exchange must handle ambient browser auth (shared client attaches
  `auth.token`) → `…-spa-route` (client) + `…-portal-contract` (server).
- [nit] `ARCHITECTURE.md` roll-forward assigned to `…-portal-contract`.

Sizing confirmed: portal-contract is large but appropriate as a feature
(feature-design splits it into stories); CLI and SPA are not too thin.

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

## Completion (autopilot, 2026-05-30)

All 3 child features done. Two major milestones, each with a full Codex xhigh
peer-review loop (per the run directive):

- **Portal contract** (done): 3 stories. Milestone review Block (consume race)
  → fixed (winner-returning consume) → confirm-clean (orphan-FK fix).
- **CLI handoff** (done): 3 stories — `--open` adopts identity, `jamsesh resume
  [session-id]`, token-safe `OpenSilent`. Durable resume works end-to-end.
- **SPA route** (done): 2 stories — `ResumeExchange` screen, `setAccessOnly`,
  public resume routes. Consumer-milestone review Block → fixes across 3
  confirm passes: playground session-view auth-gate exception; the SPA now
  sends the anonymous playground bearer (`bearerMiddleware` + WS) — a
  PRE-EXISTING foundation gap that also blocked playground *join*; and a
  non-overwrite middleware fix (signOut regression). Full Go + 799 SPA tests
  green.

End-to-end status: **durable resume fully works**; **playground resume's core
path works** (gate → session GET → WS now carry the anon bearer).

Deferred (filed as backlog, NOT resume-code defects — pre-existing):
- `cli-resolvesession-env-var-mismatch` — `ResolveSession` reads `CC_SESSION_ID`
  vs `instance_id` written from `CLAUDE_SESSION_ID` (affects finalize too).
- `playground-bearer-raw-fetch-components` — `ArtifactPane`/`ForkDialog` use raw
  `fetch`+`auth.token`, so playground participants 401 on artifact/fork (broader
  playground-SPA bearer-coverage debt; affects join too).

Foundation-doc roll-forward landed during implementation: `docs/openapi.yaml`,
`docs/SECURITY.md`, `docs/UX.md`, `plugins/jamsesh/skills/jam/SKILL.md`.
