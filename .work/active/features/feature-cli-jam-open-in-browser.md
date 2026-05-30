---
id: feature-cli-jam-open-in-browser
kind: feature
stage: drafting
tags: [plugin]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Open a jam session in the browser (`--open` flag + agent offer)

## Brief

After `jamsesh jam new` (durable or `--playground`) and `jamsesh jam join`, the
CLI prints a session URL but provides no way to actually open it. The human at
the terminal must copy-paste the URL into a browser. `finalize` and `auth`
already auto-launch the browser (`xdg-open`/`open`/`rundll32` via
`defaultOpenURL`), so the building block exists — it was simply never wired into
the session-creation/join path.

This feature adds a non-interactive `--open` boolean flag to `jam new` /
`new` and `jam join` / `join`. When set, after the session is created/joined
(push + state-write + summary print all succeed), the CLI launches the browser
at the session's portal URL. If the browser cannot launch (headless, no
`DISPLAY`, exec error), it degrades gracefully — prints the URL for the agent or
human to paste — and the command still succeeds. The `/jamsesh:jam` skill is
updated so the **agent offers** to open the session when `jam` is invoked and
passes `--open` when the user agrees; the CLI itself stays non-interactive (no
`[Y/n]` prompt).

Open targets by session kind:
- Playground (`new --playground`, `join` of a playground session) →
  `{portalURL}/playground/s/{id}/join`
- Durable (`new --org …`, `join` of a durable session) →
  `{portalURL}/orgs/{orgId}/sessions/{sessionId}` (the `session-view` route)

## Background / evidence (investigation, 2026-05-30)

- This affordance was **designed and then dropped**. The v0.4.0 story
  `cli-playground-flag.md:74-78` (now in `.work/releases/v0.4.0/`) specified the
  playground summary should end with an *"Open in browser? [Y/n]"* prompt
  reusing the `auth/` browser-open helper. The shipped
  `printPlaygroundSummary` (`cmd/jamsesh/sessioncmd/new.go:511`) printed only
  "share URL + nickname + ends-in"; the review approved without flagging the
  missing browser-open piece.
- The browser join flow itself **works** — verified live: the deployed SPA
  client-routes `/playground/s/{id}/join` to the JoinerPicker (nickname form),
  and `POST /api/playground/sessions/{id}/join` returns `200` with a bearer.
  The gap is purely the missing terminal-side open.
- Re-framed as new capability rather than restoring the literal prompt: the new
  workflow is **agent-driven / non-TTY**, so an interactive `[Y/n]` would never
  fire. Hence a flag + an agent-layer offer.

## Strategic decisions

(Locked by the user at scope time; `feature-design` should inherit these and not
re-litigate.)

- **Affordance shape**: a non-interactive `--open` flag on `jam new`/`new` and
  `jam join`/`join` — *not* a standalone `jamsesh open` subcommand and *not* a
  CLI `[Y/n]` prompt. Rationale: fits the agent-driven non-TTY flow; the offer
  lives at the agent layer.
- **Agent offers**: `plugins/jamsesh/skills/jam/SKILL.md` instructs the agent to
  offer to open the session when `jam` is invoked and to pass `--open` on the
  user's assent. The offer is conversational (the skill/agent), never a binary
  prompt.
- **Session-type scope**: both playground and durable, covering `new` (primary)
  and `join` (symmetric).
- **Failure behavior**: graceful — on any launch failure print the URL so it can
  be pasted, and return success. Reuse the existing `defaultOpenURL` semantics.

## Design inputs (for feature-design)

- **Helper reuse / promotion**: `--open` becomes the **third** consumer of the
  inlined browser opener (`auth`, `finalize`, now `new`/`join`). The note at
  `cmd/jamsesh/finalizecmd/browseropen.go:14` explicitly says to promote to a
  shared package on a third consumer. Design should extract `defaultOpenURL`
  into `cmd/jamsesh/internal/osopen` (with an injectable `openURL` function var
  for hermetic tests, mirroring finalize's indirection) and migrate `auth` and
  `finalize` to it. Confirm whether the `auth` opener and the `finalize` opener
  are byte-identical before collapsing.
- **Wiring sites**:
  - `cmd/jamsesh/sessioncmd/new.go` — add `--open` to `NewCommand` flags; honor
    it at the end of both `newAction` (durable) and `newPlaygroundAction`
    (playground), after `printSummary` / `printPlaygroundSummary`.
  - `cmd/jamsesh/sessioncmd/join.go` — add `--open` to `JoinCommand`; honor it
    after the join summary; resolve the right URL (playground vs durable
    session-view) from the resolved org/session.
  - `cmd/jamsesh/jamcmd/jam.go` — `jam` builds fresh instances of the new/join
    commands, so the flag is inherited automatically; verify it surfaces under
    `jam new`/`jam join` help.
- **URL construction**: reuse the existing share-URL builders where present
  (`printPlaygroundSummary` already computes `{baseURL}/playground/s/{id}/join`).
  Durable session-view URL shape is `/orgs/{orgId}/sessions/{sessionId}` per
  `frontend/src/lib/router.svelte.ts:21`.
- **Skill copy**: keep the `/jamsesh:jam` instructions terse; add an "offer to
  open" step and document the `--open` flag under both `jam new` and `jam join`.

## Acceptance criteria (sketch — feature-design refines + splits into stories)

1. `jam new --playground --open` opens `{portalURL}/playground/s/{id}/join`
   after the session is created and the base ref pushed.
2. `jam new --org <id> --open` opens `{portalURL}/orgs/{orgId}/sessions/{id}`.
3. `jam join <id-or-url> --open` opens the correct URL for the joined session
   (playground join URL vs durable session-view).
4. With `--open` omitted, behavior is unchanged (no browser launch).
5. Browser-launch failure prints the URL and the command still exits `0`.
6. The opener is hermetically testable via an injectable function var; no real
   browser launches in tests. `auth` and `finalize` use the same shared helper.
7. `/jamsesh:jam` instructs the agent to offer to open the session and pass
   `--open` on assent; no interactive CLI prompt is introduced.
8. `docs/UX.md` reflects the `--open` affordance in the create/join flows.

## Out of scope

- A standalone `jamsesh open [session-id]` reopen-anytime subcommand (considered
  and explicitly set aside in favor of the flag; revisit only if a "reopen a
  session I already created" need surfaces).
- Any change to the web join landing page / JoinerPicker (it already works).
- Durable-session web join flow (invite-only today; unrelated to this gap).
