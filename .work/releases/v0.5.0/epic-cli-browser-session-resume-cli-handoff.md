---
id: epic-cli-browser-session-resume-cli-handoff
kind: feature
stage: done
tags: [plugin]
parent: epic-cli-browser-session-resume
depends_on: [epic-cli-browser-session-resume-portal-contract]
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
---

# CLI resume handoff

## Brief

The CLI producer side of the handoff: from a bound session checkout, the CLI
mints a resume token (via the portal contract) using its stored session bearer,
then opens the browser to the SPA's resume route with the token in the URL
fragment. The human lands in the browser **as their CLI identity** instead of
minting a fresh playground participant or hitting an unauthenticated durable
view.

Works for both playground and durable sessions (the mint call is the same; the
portal decides the credential at exchange time). Reuses the
`cmd/jamsesh/internal/osopen` browser opener shipped in
`feature-cli-jam-open-in-browser`, and the per-session bearer in CLI state.

Does NOT implement the portal endpoints (sibling `…-portal-contract`) or the
browser-side exchange (sibling `…-spa-route`).

## Epic context

- Parent epic: `epic-cli-browser-session-resume`
- Position in epic: consumer of `…-portal-contract`. Independent of
  `…-spa-route` (different component) — the two consumers can be designed and
  built in parallel once the contract lands.

## Foundation references

- `cmd/jamsesh/sessioncmd/new.go` / `join.go` — where the existing `--open`
  flag + `openInBrowser` / URL builders live; the resume handoff slots in
  alongside.
- `cmd/jamsesh/internal/osopen/` — the browser opener to reuse.
- `cmd/jamsesh/state/` — per-session bearer + `org_id`/`session_id` sidecars the
  mint call needs.
- `cmd/jamsesh/portalclient/` — the client to call `POST /api/session-resumes`.

## Design decisions

(Captured in the questions-only alignment pass, 2026-05-30. CLI-surface choice
cross-referenced with Codex — see `## Other agent consult`.)

- **CLI surface = (2) + (3)**: `jam new --open` / `jam join --open` **adopt the
  CLI identity by default** (the session is unambiguous at create/join time),
  AND add a dedicated **`jamsesh resume [session-id]`** subcommand for the
  reopen-later case. Bare `jamsesh resume` targets the `ResolveSession()`
  current CC-instance session; an explicit `<session-id>` disambiguates;
  `jamsesh status` is the lister. — Mirrors the `kubectl`/`aws --profile`/`gh`/
  `fly` "act-on-current-context + explicit selector + status lister" convention.
- **No silent wrong-session resume** [footgun]: bare `jamsesh resume` must only
  use the `CC_SESSION_ID`→session mapping; if `ResolveSession` would fall back
  to "first dir" while multiple sessions exist, ERROR with `jamsesh status`
  guidance rather than guess.
- **No silent identity-adoption degrade** [footgun]: if `--open` cannot mint a
  resume token (e.g. portal lacks support, mint fails), either fail with a clear
  message or open token-free WITH an explicit warning — never silently fall back.
- **`--open` meaning change accepted**: `--open` now means "open this session in
  the browser *as me*" (was token-free). This modifies the behavior of the
  `--open` flag shipped in `feature-cli-jam-open-in-browser`; that's in-scope for
  this epic. (Not a contradiction of that feature's flag-vs-subcommand choice —
  `resume` is a distinct reopen-later/multi-session need, not a second "open".)
- **Token-leak rule still applies**: the resume URL (with `rt` fragment) must
  never be printed (see Decomposition-review findings); the CLI opens the
  portal-returned `resume_url` without echoing it.

The full design pass settles: exact flag wiring, the `resume` subcommand
structure, session-selection UX, and how `--open` detects "resume possible".

## Other agent consult

Codex CLI-surface consult (2026-05-30): recommended (2)+(3) over (1)+(3)
("`--resume` on new/join is awkward — you're not resuming an existing session,
you're opening the just-made one as yourself") and over (2)-alone ("doesn't
solve reopen-the-right-one-later"). Footguns above are its accepted findings.

## Superseded open question

- ~~CLI surface: `--open` vs `--resume` vs `jamsesh resume`~~ — resolved above.

## Decomposition-review findings (Codex, accepted — fold into this feature's design)

- **Do NOT leak the resume token to terminal scrollback.** [BLOCKER] The resume
  URL carries the single-use token in its fragment, and the existing
  `openInBrowser` (`cmd/jamsesh/sessioncmd/new.go`) *prints* `Opening in
  browser: <url>`. This feature must NOT reuse that helper verbatim for the
  resume URL — print a token-free message (e.g. the bare session URL, or
  "Opening your session in the browser…"), and the open-failure fallback must
  also avoid printing the tokened URL. Add explicit acceptance criteria for
  redacted/no-print resume URLs.
- **Open the portal-returned `resume_url` VERBATIM — do not build it.** Mint
  returns the fully-formed `resume_url` (fragment + canonical path); the CLI
  opens that string and never constructs the route itself (eliminates drift).
  The only CLI-built display text is token-free (e.g. the bare session URL).
- **Durable mint** must send `org_id` + `session_id` so the portal can do the
  membership check (the durable CLI bearer is account-scoped, not session-bound).

## Foundation-doc roll-forward (at implementation)

`docs/UX.md` (the resume step in the create/join CLI flows); the `/jamsesh:jam`
skill if a new flag/command surfaces.

## Other agent review (Codex xhigh advisory, 2026-05-30)

Accepted points folded into the design below:
- **Token safety is the highest risk.** Do NOT reuse the existing
  `openInBrowser`/`osopen.Open` seam for the resume URL — `osopen.Open` PRINTS
  the raw URL on launch failure (and on unsupported OS), leaking the `#rt=`
  fragment token. A token-safe opener that never prints the secret URL is
  required (Unit 1).
- **Client credential class.** `portalclient.Client` only reads the per-session
  bearer when `Client.SessionID` is set; otherwise it uses the legacy
  account-token path. The mint client MUST set `SessionID` so the per-session
  bearer is used — this is how playground (anon bearer) AND durable both
  authenticate correctly. Do NOT use `buildPortalClient()` for the playground
  mint.
- **Failure semantics differ**: `--open` mint failure → warn + fall back to the
  old token-free open (create/join already succeeded). Standalone `jamsesh
  resume` mint failure → ERROR, open nothing (identity adoption is its whole
  purpose).
- **Bare `resume` resolution**: pre-existing `ResolveSession` env mismatch
  (backlog `cli-resolvesession-env-var-mismatch`) — use the write-consistent
  resolver `state.CurrentSessionID` (`CLAUDE_SESSION_ID`) for bare resume; if a
  CC instance env is present but unmapped, require an explicit id even with one
  session; outside CC context with exactly one session, resume it.
- **Footguns**: generated fields are `OrgId`/`SessionId`/`ResumeUrl`; no `--json`
  for resume in v1 (token-exposure risk); no portal preflight (special-case
  404/405 → "portal doesn't support resume handoff"); expiry message "expires in
  60s" but NEVER print the link; browser-argv fragment exposure is an inherent
  local risk mitigated by the 60s single-use TTL.

## Architectural choice

A **token-safe opener** + a shared **mint-and-open** helper, consumed by both
the `--open` adoption path (new/join) and the new `resume` subcommand. The mint
client is constructed with `SessionID` set so the per-session bearer
authenticates the mint for both credential classes. Three stories split by
failure-semantics/risk (Codex's cut), not just by shared code.

## Implementation Units

### Unit 1: token-safe opener + mint helper + `--open` adoption
**Story**: `epic-cli-browser-session-resume-cli-handoff-mint-open-adopt`
**Files**: `cmd/jamsesh/internal/osopen/osopen.go` (+ test),
`cmd/jamsesh/sessioncmd/resume.go` (new: shared helper), `new.go`, `join.go` (+ tests)

```go
// osopen: a token-safe variant that NEVER writes the URL anywhere (no
// print-on-failure), for URLs carrying secrets in the fragment.
func OpenSilent(rawURL string) error   // launches; returns err on failure; prints NOTHING

// sessioncmd: shared mint+open helper. pc MUST be built with SessionID set so
// the per-session bearer (anon or durable) authenticates the mint.
func mintAndOpenResume(ctx context.Context, pc *portalclient.Client, orgID, sessionID string) error
//  1. resp := PostJSON[openapi.SessionResumeResponse](ctx, pc, "/api/session-resumes",
//        openapi.SessionResumeRequest{OrgId: orgID, SessionId: sessionID})
//  2. validate resp.SessionId == sessionID AND resp.ResumeUrl != "" BEFORE opening (else error, open nothing)
//  3. fmt.Println("Opening your session in the browser (resume link expires in 60s)…")  // token-free
//  4. return osopen.OpenSilent(resp.ResumeUrl)  // never prints the URL
```

`--open` adoption (replaces the token-free open at the existing `cmd.Bool("open")`
sites): `newAction` (durable), `newPlaygroundAction` (playground), `joinAction`
each build a `pc` with `SessionID` set and call `mintAndOpenResume`; on error,
print a one-line WARNING and fall back to the previous token-free
`openInBrowser(sessionViewURL/playgroundJoinURL)` behavior.

**Acceptance**:
- [ ] `--open` (durable, playground, join) mints then opens the exact
      `resp.ResumeUrl`; uses the per-session bearer (playground → anon, not OAuth).
- [ ] `OpenSilent` and the mint/open path NEVER write `#rt=` (or the resume_url)
      to stdout/stderr — on success, on mint failure, AND on browser-open failure.
- [ ] Empty `ResumeUrl` or `SessionId` mismatch → error before opening.
- [ ] `--open` mint failure → warning + token-free fallback open (old behavior).
- [ ] Tests override the open seam + use an httptest portal for mint.

### Unit 2: `jamsesh resume [session-id]` subcommand
**Story**: `epic-cli-browser-session-resume-cli-handoff-resume-command`
**Files**: `cmd/jamsesh/sessioncmd/resume.go`, `cmd/jamsesh/main.go` (register)

`ResumeCommand()` — arg `[session-id]` optional. Resolve: explicit id → that
session; bare → `state.CurrentSessionID()` (write-consistent / `CLAUDE_SESSION_ID`,
NOT `ResolveSession` — see backlog `cli-resolvesession-env-var-mismatch`); if a
CC-instance env is present but unmapped → error with a `jamsesh status` hint;
outside CC context with exactly one session → resume it; multiple sessions +
unmapped → error + `jamsesh status` hint. Read `org_id` from session state, build
`pc` with `SessionID` set, call `mintAndOpenResume`. Mint failure → ERROR
(nonzero exit), open nothing.

**Acceptance**:
- [ ] `jamsesh resume <id>` mints+opens for that session; `jamsesh resume`
      (bare) resolves the current-instance session.
- [ ] Multiple sessions + unmapped instance → error citing `jamsesh status`,
      opens nothing.
- [ ] Mint failure → nonzero exit, nothing opened, no token printed.
- [ ] Registered as a top-level command in `main.go`.

### Unit 3: skill + docs roll-forward
**Story**: `epic-cli-browser-session-resume-cli-handoff-skill-docs`
**Files**: `plugins/jamsesh/skills/jam/SKILL.md`, `docs/UX.md`

Document `--open` now adopts identity + the `jamsesh resume [session-id]`
command; UX.md resume step in create/join flows. Present-tense; describes
shipped behavior.

## Implementation Order

1. Unit 1 (`…-mint-open-adopt`) — depends on: `[]` (portal contract is done)
2. Unit 2 (`…-resume-command`) — depends on: `[…-mint-open-adopt]` (uses the helper + opener)
3. Unit 3 (`…-skill-docs`) — depends on: `[…-resume-command]` (docs the shipped surface)

## Testing

- Unit 1: override the `openURL`/`OpenSilent` seam to capture the opened URL +
  assert NO `#rt=` ever reaches stdout/stderr (the security AC); httptest portal
  serving `POST /api/session-resumes`; assert playground path uses the anon
  per-session bearer (not OAuth); `--open` mint-failure → warn + token-free open.
- Unit 2: resolver matrix (explicit id, bare-current, multi-session-unmapped →
  error+hint); mint-failure → nonzero + nothing opened.
- Unit 3: reviewer confirms copy.

## Risks

- **Token leakage via the opener** (Unit 1) is the headline risk — the
  `OpenSilent` path and every failure branch must be silent about the URL. The
  test asserting "no `#rt=` in any output" is the guard.
- **Wrong credential class**: if the mint client doesn't set `SessionID`, the
  legacy account-token path breaks playground mint. The design pins
  `SessionID`-set client construction.
- Bare-resume resolution rides on the parked `ResolveSession` env mismatch —
  mitigated by using `state.CurrentSessionID`; the parked item fixes the root.
