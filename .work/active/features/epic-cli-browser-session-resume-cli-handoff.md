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

## Open design question (deferred to this feature's design pass)

- **CLI surface**: extend `--open` to resume-when-possible, vs a dedicated
  `--resume` flag, vs a `jamsesh resume` subcommand. (This is the kind of
  feature-level question the questions-only design pass will settle.)

## Decomposition-review findings (Codex, accepted — fold into this feature's design)

- **Do NOT leak the resume token to terminal scrollback.** [BLOCKER] The resume
  URL carries the single-use token in its fragment, and the existing
  `openInBrowser` (`cmd/jamsesh/sessioncmd/new.go`) *prints* `Opening in
  browser: <url>`. This feature must NOT reuse that helper verbatim for the
  resume URL — print a token-free message (e.g. the bare session URL, or
  "Opening your session in the browser…"), and the open-failure fallback must
  also avoid printing the tokened URL. Add explicit acceptance criteria for
  redacted/no-print resume URLs.
- **Use the contract-owned route shape + `rt` fragment key** (defined in
  `…-portal-contract`) when building the URL — don't invent a divergent shape.
- **Durable mint** must send `org_id` + `session_id` so the portal can do the
  membership check (the durable CLI bearer is account-scoped, not session-bound).

## Foundation-doc roll-forward (at implementation)

`docs/UX.md` (the resume step in the create/join CLI flows); the `/jamsesh:jam`
skill if a new flag/command surfaces.
