---
id: story-epic-ephemeral-playground-session-lifecycle-cli-playground-flag
kind: story
stage: done
tags: [plugin, playground]
parent: feature-epic-ephemeral-playground-session-lifecycle
depends_on: [story-epic-ephemeral-playground-session-lifecycle-rest-endpoints]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# `jamsesh new --playground` CLI extension

## Scope

Story 4 of the parent feature. Extends the `jamsesh new` subcommand
(owned by wave-1 `cli-first-creation` feature) with a `--playground`
flag. When set, the subcommand:
1. Skips auth (no OAuth bearer required)
2. Skips org picker (creator joins the reserved `playground` org)
3. Calls `POST /api/playground/sessions` instead of `POST /api/orgs/{id}/sessions`
4. Writes the bearer received in the response to per-session token storage
5. Pushes local HEAD as base ref using the just-received bearer
6. Prints a playground-specific success summary (share URL + nickname +
   ends-in counter)

Full design in the parent feature body's "Story 4" section.

## Files delivered

- `cmd/jamsesh/sessioncmd/new.go` (modify, originally created by
  wave-1 `cli-first-creation`):
  - Add `&cli.BoolFlag{Name: "playground", ...}` to NewCommand flags
  - Branch early in `newAction`: if `--playground` set, dispatch to
    `newPlaygroundAction`
  - Add `newPlaygroundAction(ctx, cmd) error`
  - Add `pushBaseRefWithBearer(ctx, pc, sessionID, bearer)` — variant
    of wave-1's `pushBaseRef` that uses an explicit bearer rather than
    reading from `state.ReadToken()`
  - Add mutual-exclusion guard: `--playground` + `--org` returns error
- `cmd/jamsesh/sessioncmd/new_test.go` (extend) — playground-path tests
- `cmd/jamsesh/state/state.go` (modify if not already done by wave-3
  `plugin-skills`) — `WriteSessionToken(sessionID, bearer)` helper that
  writes to `${CLAUDE_PLUGIN_DATA}/sessions/<id>/token`

## Acceptance criteria

See the parent feature body's "Story 4 acceptance criteria" section.
Summary: `jamsesh new --playground` creates via the playground endpoint
(no auth), pushes HEAD with the just-received bearer, writes per-session
state, prints share URL + nickname + ends_at; mutually exclusive with
`--org`.

## Notes for the implementing agent

- This story extends a file owned by wave-1 cli-first-creation
  (`cmd/jamsesh/sessioncmd/new.go`). When wave-1 implements first
  (which it does — it's wave 1, this is wave 2), the file exists with
  the durable-path code. This story ADDS to it; doesn't rewrite.
- The `WriteSessionToken` helper is also referenced by wave-3
  `plugin-skills` feature (which owns the unified per-session bearer
  storage migration). If `plugin-skills` lands first (wave 3 vs wave 2
  — won't happen in practice), reuse its helper; otherwise this story
  introduces it and `plugin-skills` extends.
- `pushBaseRefWithBearer` is a thin variant of the wave-1
  `pushBaseRef`: same `-c http.extraHeader=...` token-injection
  approach (don't switch to URL-embedded credentials), but takes the
  bearer as a parameter rather than reading from
  `state.ReadToken()`. After Story 4 lands, the wave-1 `pushBaseRef`
  could refactor to call `pushBaseRefWithBearer(ctx, pc, sessionID,
  state.ReadToken())` — but that refactor is out of scope here.
- The success summary should match the visual feel of the parent epic's
  mockup step `02-create-cli-output.html`: share URL, "You are:
  <nickname>", "Ends: in <hard-cap-remaining> (or after <idle-remaining>
  idle)", "Open in browser? [Y/n]" prompt. The browser-open piece can
  reuse the existing `cmd/jamsesh/auth/` browser-open helper.

## Cross-story notes

- Depends on Story 1 (REST endpoints) because this story calls the
  endpoints Story 1 creates.
- Depends conceptually on wave-1 cli-first-creation having landed
  (the `new.go` file must exist). Cross-feature deps are inherited
  via the feature-level depends_on declared in the parent feature's
  frontmatter — no need to re-declare here at the story level.

## Implementation notes

### Files changed

- `cmd/jamsesh/portalclient/client.go` — added `PostJSONAnon[T]()`, a
  generic that issues an unauthenticated POST (no Authorization header),
  parallel to the existing `GetJSONWithBearer` pattern already in the file.

- `cmd/jamsesh/sessioncmd/new.go` — extended as designed:
  - Added `--playground` `BoolFlag` to `NewCommand`
  - Mutual-exclusion guard at the top of `newAction` (`--playground` + `--org` → clear error)
  - Early branch to `newPlaygroundAction` when `--playground` is set
  - `buildPlaygroundClient()` — returns portal URL without requiring a stored OAuth token
  - `newPlaygroundAction(ctx, cmd)` — calls `PostJSONAnon` → stores bearer → pushes → writes state → prints summary
  - `pushBaseRefWithBearer(ctx, baseURL, sessionID, bearer)` — thin variant of wave-1 `pushBaseRef` using an explicit bearer; same `-c http.extraHeader=` approach (no URL-embedded credentials)
  - `writePlaygroundSessionState(session)` — writes org_id (`"org_playground"`), ref, last_seen_seq; no account_id (anonymous)
  - `wrapPlaygroundPushError` — push-failure wrapper with retry command
  - `printPlaygroundSummary` — share URL + nickname + ends-in

- `cmd/jamsesh/sessioncmd/new_test.go` — 4 new playground tests:
  - `TestPlaygroundAction_happyPath` — full happy path; asserts no auth header on create, push fires, state files written, token stored
  - `TestPlaygroundAction_namePassthrough` — `--name "demo"` propagated to request body
  - `TestPlaygroundAction_mutuallyExclusiveWithOrg` — `--playground --org foo` returns error mentioning "mutually exclusive"
  - `TestPlaygroundAction_pushUsesBearerNotOAuthToken` — push `http.extraHeader` contains the Base64-encoded anon bearer, not the OAuth token

### Design discoveries

- `WriteSessionToken` was already implemented by the bearer-storage story (confirmed in `state.go`). No changes needed to `state.go`.
- `portalclient.Client.Do` always calls `attachBearer` (reads state OAuth token), making it unsuitable for anonymous playground calls. Added `PostJSONAnon` as a bare `net/http` call with no auth header, following the existing `GetJSONWithBearer` pattern.
- `buildPlaygroundClient()` returns `(string, *http.Client, error)` rather than `*portalclient.Client` to keep the anonymous path clearly separate from the authenticated client — no risk of accidentally wiring token refresh on playground calls.
- `net/http` import added to `new.go` (was not previously imported; only `net/url` was present).

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `cmd/jamsesh/sessioncmd/new.go` (`printPlaygroundSummary`): design spec
  for the summary line was "Ends: in <hard-cap-remaining> (or after
  <idle-remaining> idle)". Implementation prints "Ends: in <hard-cap>
  (hard cap) or after idle timeout" — the hard-cap remaining is shown
  but the per-session idle-remaining number is not (despite
  `IdleTimeoutAt` being available on the response). Both pieces of
  information are conveyed semantically; the numeric value for idle is
  the only thing missing. Tiny copy/visual nit, not worth a follow-up
  unless the design wants the literal idle minutes surfaced.
- `pushBaseRefWithBearer` (and the wave-1 `pushBaseRef` it mirrors)
  capture `headSHA` into a discarded local — the `_ = headSHA` line
  documents that this is "validation, not consumption". Fine as-is;
  any future refactor could drop the variable entirely since
  `runGitOutput("rev-parse", "HEAD")` already serves as a validation
  call.

**Notes**:
- All 5 AC items from the parent feature's Story 4 verified:
  1. Creates via `/api/playground/sessions` with no Authorization
     header — `TestPlaygroundAction_happyPath` asserts both calls and
     the absence of the auth header.
  2. `--name "demo"` propagates to the request body —
     `TestPlaygroundAction_namePassthrough`.
  3. `--playground` without `--org` does not prompt — implicit in the
     happy-path test (no prompt machinery touched).
  4. Mutually-exclusive guard fires before any network call —
     `TestPlaygroundAction_mutuallyExclusiveWithOrg`; error string
     contains "mutually exclusive".
  5. Push uses the received anon bearer via `-c http.extraHeader=`,
     not URL-embedded credentials, not the OAuth token —
     `TestPlaygroundAction_pushUsesBearerNotOAuthToken` asserts the
     bearer's Base64 form is present and any OAuth token is absent.
- Build clean (`go build ./...`), vet clean (`go vet
  ./cmd/jamsesh/sessioncmd/ ./cmd/jamsesh/portalclient/`),
  sessioncmd/portalclient/state package tests pass.
- Design deviation `PostJSONAnon` vs design's `portalclient.PostJSON`
  is principled: keeping the anonymous path off the
  `portalclient.Client.Do` codepath eliminates accidental wiring of
  `attachBearer` / future token-refresh logic onto playground calls.
  Documented under "Design discoveries".
- Design deviation `buildPlaygroundClient` returns `(baseURL,
  *http.Client, err)` rather than a `*portalclient.Client` — same
  rationale; keeps the anonymous surface explicitly distinct from the
  authenticated one. Also documented.
- Push-failure behavior matches the locked decision: the session
  stays live with `base_sha: null` and the user gets a retry command
  pointing at the same remote URL (`wrapPlaygroundPushError`).
- Per-session state files written (`token`, `org_id`, `ref`,
  `last_seen_seq`) match the per-session storage layout already
  established by the wave-3 plugin-skills bearer-storage migration
  (`WriteSessionToken` was reused, not redefined).
- Foundation docs (`docs/UX.md:93`, `docs/SPEC.md`, `docs/SECURITY.md`,
  `docs/openapi.yaml`) already cover `jamsesh new --playground` and
  `POST /api/playground/sessions`; no doc roll-forward needed.
