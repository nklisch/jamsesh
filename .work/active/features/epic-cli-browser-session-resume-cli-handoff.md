---
id: epic-cli-browser-session-resume-cli-handoff
kind: feature
stage: drafting
tags: [plugin]
parent: epic-cli-browser-session-resume
depends_on: [epic-cli-browser-session-resume-portal-contract]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
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
