# Changelog

## v0.4.0

Released 2026-05-24. The headline is the **ephemeral anonymous
playground**: anyone can spin up a throwaway jam session — no account,
no org membership, no setup — collaborate with up to N participants on a
short-lived bare-repo workspace, then watch the destruction worker tomb
it on idle-timeout or hard-cap. Behind the scenes that pulled in a new
anonymous-bearer auth surface, a reserved `playground` org guarded by
`org_protected`, REST + WebSocket lifecycle endpoints, a destruction
sweep worker, a CLI `jamsesh jam` flow, a slash-command consolidation
(`/jamsesh:jam` + `/jamsesh:finalize`), and a from-scratch SPA anonymous
entry surface. Alongside the playground epic, five large refactors
landed (adapter dialect dedup, generic `wrap` helpers, auto-merger
decomposition, frontend god-components split into seams, per-package
clock compliance) and the spec-discipline feature wired up a drift CI
check plus moved inline event-type strings to generated openapi-fetch.
Five quality gates (security, tests, cruft, docs, patterns) ran on the
bundle; findings drained in-cycle except for items explicitly parked to
backlog. Five new pattern skills shipped under `.claude/skills/patterns/`.

### Features

- **Ephemeral anonymous playground** — `POST /api/playground/sessions`
  spins up a throwaway jam without auth, server-mints a pronounceable
  handle + an anonymous bearer scoped to that session, and bare-repo
  creation happens after the session-row TX commits (3-step sequence
  to avoid SQLite WAL deadlock; partial failure leaves an orphan that
  the destruction sweep cleans up). Joiners arrive via `POST
  /api/playground/sessions/{id}/join` with an optional 2-24 char
  letters/digits/dashes nickname (server-side validated as of this
  release). A reserved `playground` org is guarded by an `org_protected`
  flag — booting the portal with `--playground-enable` against a DB
  where the slug is taken by a non-protected org exits 1 instead of
  silently squatting. Lifecycle is bounded by configurable `IdleTimeout`
  (default 30m) and `HardCap` (default 2h); abuse is bounded by a
  per-IP/hour create cap and a per-session content cap enforced at
  pre-receive time. Tombstones with member/commit/auto-merge counts
  outlive the destroyed session for a short TTL so late HTTP polls get a
  meaningful 200 instead of a bare 404. Closes
  `epic-ephemeral-playground` (1 epic, 7 features, 15+ stories).
- **`jamsesh jam` CLI command and `/jamsesh:jam` slash command** — one
  command for "start or attach to a jam." On first use without a session
  id, it pushes the working tree's base-sha + creates a playground
  session via the same anonymous REST surface; with an id, it attaches
  to an existing session, optionally as a joiner picker. The plugin
  slash commands consolidate from the previous fan-out into two
  surfaces: `/jamsesh:jam` (start/join/attach) and `/jamsesh:finalize`
  (finish the jam). A new `destruction-warning` skill warns the agent
  when the session is near its hard-cap or idle-timeout boundary; a new
  `status-enumeration` skill normalizes how status is reported across
  jam-aware tools. Closes
  `feature-epic-ephemeral-playground-cli-first-creation` and
  `feature-epic-ephemeral-playground-plugin-skills`.
- **SPA anonymous-entry surface** — the Home screen now has an explicit
  "Try a playground" path that walks through anonymous session creation
  without forcing a sign-in detour. The session view picks up a
  playground branch (clock-driven hard-cap reason rendering, a
  destruction-warning banner, a joiner picker that handles the 410
  session-ended race), the new-session drawer was reworked, and the
  router was refactored so anonymous routes no longer share guards with
  authenticated routes. Closes
  `feature-epic-ephemeral-playground-portal-ui`.
- **Spec-discipline CI gate** — `docs/openapi.yaml` is now the
  load-bearing single source of truth for event-type strings;
  `events.AllTypes` mirrors the `EventEnvelope.type` enum exactly and a
  `TestEventTypeConstants_MatchOpenAPIYAML` regression test plus a
  drift CI check enforce both directions on every push. SPA event types
  were migrated from hand-maintained `as const` strings to
  `openapi-typescript`-generated types. Closes `feature-spec-discipline`.

### Fixes

- **Magic-link request rejects `@playground.local` domains** — the
  anonymous-bearer subsystem creates synthetic accounts with
  `anon_<random>@playground.local` emails for internal bookkeeping;
  `POST /api/auth/magic-link/request` now rejects any user-supplied
  address in a reserved domain with HTTP 400 and
  `magic_link.reserved_domain` so an attacker can't request a magic link
  for an anonymous account by guessing the synthetic email shape.
- **Plugin slash references rolled forward** — three stories
  (`skill-consolidation-primer-stale-slash-refs`,
  `skill-consolidation-references-stale-slash-refs`,
  `skill-consolidation-rollforward-foundation-docs`) swept stale
  `/jamsesh:join` / `/jamsesh:create` / etc. references in skill
  primers and foundation docs to the post-consolidation
  `/jamsesh:jam` + `/jamsesh:finalize` shape.
- **`orgs` handler now wraps DB auth failures with `deperr`** — auth-fail
  branches were returning raw store errors, which bypassed the
  `httperr.WriteFromError` classifier and surfaced as 500s instead of
  the typed dep-unavailable envelope.
- **Tombstone-expired vs session-active 404 disambiguation** —
  `GET /api/playground/sessions/{id}/tombstone` now distinguishes "no
  such tombstone yet (session may still be live)" from "tombstone TTL
  has elapsed" in the 404 message so clients polling for destruction
  status don't conflate the two states.
- **Reserved-slug conflict at portal boot** — booting with
  `--playground-enable` against a DB where the `playground` slug is
  taken by a non-protected org now exits 1 with a clear error instead
  of silently overwriting the reserved-org wiring.

### Security

- **Defense-in-depth on `org_protected`** — the `org_protected` flag now
  guards not just slug squatting at boot but also policy-mutation
  endpoints, so an authenticated user can't reach into the
  reserved-org's policy surface via the standard org-management routes.
- **Pre-receive `writable_scope` validation** — playground session
  creation validates the supplied `writable_scope` at the REST front
  door using the same `prereceive.ValidateWritableScope` check the git
  HTTP layer runs, so a malformed scope is rejected as 400 instead of
  surfacing later as a deferred pre-receive failure that leaves a
  poisoned session behind.
- **Bearer-issuance partial-failure orphan recovery** — the
  3-step CreatePlaygroundSession sequence (session TX → bearer issuance
  → member insert) now has a regression test that injects a failing
  tokens.Service, asserts the orphaned session row persists, and
  asserts the destruction worker cleans it up.

### Refactor

- **Auto-merger decomposed** — the auto-merger's god-function was split
  into `side-changes`, `both-modified`, `merge-file`, and
  `flatten-submodule` helpers, each with its own focused test surface;
  no behavior change. Closes `feature-refactor-automerger-decomposition`.
- **Frontend god-components split into seams** — `FinalizeView`,
  `SessionViewShell`, `SessionAttachWalkthrough`, `JoinerPicker`,
  `NewSessionDrawer`, and `OrgSettings` were broken out into smaller
  components and rune-store hooks (`useNewSessionForm.svelte.ts`,
  `useFinalizeExecution.svelte.ts`) with explicit seam contracts
  asserted by a new `gate-tests-frontend-god-components-seam-contracts`
  story. Closes `feature-refactor-frontend-god-components`.
- **Per-package Clock interface compliance** — four more packages
  (`ratelimit`, `lease`, `auth`, `objectstore`) adopted the per-package
  `Clock interface{Now() time.Time}` + `realClock{}` fallback pattern so
  `*testclock.AdvanceableClock` can advance them without import
  coupling. Brings the in-use count to 14 packages. Closes
  `feature-refactor-per-package-clock-compliance`.
- **Adapter `wrap1` / `wrapList` generics** — single-row and list
  adapter methods in `internal/db/store/{sqlite,postgres}_adapter.go`
  collapse to one line via package-private generic helpers in
  `wrap.go`. Closes `feature-refactor-adapter-generic-wrap-helpers`.
- **Adapter dialect dedup of null-value converters** — the per-row
  `sql.Null{String,Int64,Time}` ⇄ `*T` converters were colocated and
  deduplicated across the sqlite/postgres adapters. Closes
  `feature-refactor-adapter-dialect-dedup`.
- **Router deps struct split** — `combinedHandler` construction was
  refactored from a 30-arg constructor into a deps struct; openapi
  strict-server handlers now compose through a narrower interface.
- **`events.Log.Emit` batch helper extracted** — the
  emit-event-then-fanout pattern was deduped into a single helper used
  by every domain mutation path.
- **`config.Validate` and env-helper extraction** — the portal main's
  config loading was extracted into focused validate / env-helper
  functions so the main wiring stays declarative.
- **Per-session read-token sweep** — `state.readtoken` sweep is now
  per-session instead of global, reducing the surface scanned during
  routine cleanup. Closes `feature-state-readtoken-per-session-sweep`.

### Documentation

- **Foundation docs rolled forward for the playground epic** —
  `docs/SPEC.md`, `docs/SECURITY.md`, `docs/PROTOCOL.md`,
  `docs/ARCHITECTURE.md`, and `docs/UX.md` all picked up playground
  surfaces: REST routes, idle-timeout / hard-cap config, anonymous
  bearer storage shape, reserved-org guard, destruction worker,
  destruction-warning protocol event, and the spin-up / join flows.
- **`README.md` rewritten for `/jamsesh:jam` + `/jamsesh:finalize`** —
  the slash-command list and Claude Code quick-start were updated to
  the post-consolidation shape.
- **Five new pattern skills** in `.claude/skills/patterns/`:
  `per-instance-factory-rune-store`, `adapter-wrap-helpers`,
  `strict-server-partial-handler-shim`, `playground-activity-reset`,
  and `ticker-sweep-loop`. Three existing pattern skills
  (`per-package-clock-interface`, `dual-dialect-mirror-queries`,
  `openapi-fetch-middleware-client`) were rolled forward for v0.4.0
  reality (package counts, column lists, symbol-based anchors).
- **`docs/openapi.yaml` documents the playground 400 paths** —
  `playground.invalid_writable_scope`, `playground.invalid_nickname`,
  and `magic_link.reserved_domain` are now first-class documented error
  envelopes.

### Internal

- **Anonymous-bearer test integrity** — a new
  `feature-anon-bearer-test-integrity` shipped a `migration-updownup`
  round-trip test for the schema-17→18 changes and a
  `transactional-rollback` test for the bearer-issuance TX.
- **Gate-cruft sweep** — 7 dead-code items drained (orphaned
  `noopLogger`, unused `stepClock`, orphaned `countingHydrator`,
  test-only `parsePackedRefsContent`, unused `beforeEach` import, unused
  `WARN_THRESHOLD_MS` const, dead `_ = time.Second` line).
- **`comments.Service` uses `slog`** — replaced the last stdlib
  `log.Printf` call with the project-standard `slog`.
- **CLI `parseInviteEmails` dedupe regression test** added.
- **80 substrate items bound to v0.4.0**; 40 medium-severity drafting
  items deferred to subsequent releases. Pre-existing failing tests
  `TestJoinPlaygroundSession_Success` and
  `TestJoinPlaygroundSession_WithNickname_UsesIt` were parked as
  `bug-playground-join-with-nickname-returns-410-on-fresh-session` for
  a dedicated fix cycle — not bundled into this release per
  test-integrity discipline.

## v0.3.1

Released 2026-05-21. Patch release adding the portal's session-attach
onboarding — users finally get told how to actually attach a Claude Code
instance to a jam after creating or accepting an invite — plus a refresh
of the in-plugin SKILL.md guidance for distributed agent consumers.

### Features

- **Portal session-attach onboarding** — new `SessionAttachWalkthrough`
  modal appears after a successful session create (in `SessionList` via
  `NewSessionDrawer`) and after accepting an invite (in `InviteAccept`),
  walking the user through the three commands needed to attach: `claude
  plugin marketplace add nklisch/jamsesh`, `claude plugins install
  jamsesh`, then `/jamsesh:join <session-id>` inside Claude Code. The
  modal has two states — full ceremonial walkthrough for first-time
  users, compact one-line view (just the join command) for returning
  users. Persists "don't show again" per-browser via localStorage. An
  always-reachable `AttachHelpLink` ("Setup help") lives in the
  `SessionList` and `SessionViewShell` chrome for users who dismissed
  the full walkthrough but need to re-reference the commands. The CC
  pane inside the modal mimics Claude Code's actual TUI chrome (real
  `claudecode-color.svg` icon, slate-navy bg, `#D97757` clay accent,
  `❯` prompt indicator) so the surface distinction (your terminal vs.
  Claude Code) is unmistakable. Closes
  `feature-portal-session-attach-onboarding` (feature, 5 stories).

### Documentation

- **`skills/jamsesh/SKILL.md` refocused for distributed agent consumers**
  — guidance the CC plugin auto-loads is now written from the agent's
  point of view rather than the plugin author's. Affects every CC
  instance running the jamsesh plugin (commit `196518c`).

### Internal

- **bats wrapper test fix** — `tests/wrapper/install.bats:43` hardcoded
  `v0.2.0` in the cached-binary path assertion, which silently broke
  after the v0.3.0 release bumped `bin/jamsesh`'s
  `JAMSESH_PLUGIN_VERSION`. Test now reads the version from the wrapper
  at test time so it tracks future bumps automatically (commit
  `33258ce`).
- 4 backlog ideas parked during the cycle for later scoping
  (`idea-ephemeral-jam-playground` and others) — internal substrate
  housekeeping, not user-visible.

## v0.3.0

Released 2026-05-20. The SPA gets its first real authenticated landing
surface: after sign-in, users see their orgs and either auto-route into
the single-org case or pick from a list. The `bin/jamsesh` wrapper gets
a regression harness so the multi-arch download flow can't silently
break. Security and test-coverage gates ran on the bundle and produced
21 items (all drained), tightening the 401-blanket-signout path,
adding scheme/host validation to the OAuth redirect, and pinning a
handful of behavioral contracts that were spec'd but not tested.

### Features

- **Logged-in landing screen and org bootstrap** — after authenticating,
  the SPA now hydrates `/api/me` once, caches the user + org membership in
  the auth rune store, and renders one of three states: loading (orgs
  null), empty (zero orgs → "create your first org" CTA), or picker
  (2+ orgs). Single-org accounts auto-route to
  `/orgs/<id>/sessions`. The auth store gained `currentUser`, `orgs`,
  `loadCurrentUser()`, and `addOrg()`, plus a cross-tenant guard that
  discards stale `/api/me` responses if `signOut` raced the call.
  `Login.svelte` and `OAuthCallback.svelte` redirect into the new
  surface. Closes `spa-logged-in-landing-and-org-bootstrap` (feature,
  3 stories).
- **`bin/jamsesh` regression harness** — a bats test suite plus CI job
  exercises the wrapper's binary-fetch + cache path end-to-end on every
  push. Catches platform-tarball regressions and `/var/cache/jamsesh/`
  layout drift before they reach users. Closes
  `testing-bin-jamsesh-regression-harness` (feature, 2 stories).
- **Claude Code plugin install instructions in README** — verified
  `claude plugin marketplace add nklisch/jamsesh` and
  `claude plugins install jamsesh` against the live CC CLI; section
  sits between "Operator quickstart" and "License". Closes
  `docs-readme-cc-plugin-install-instructions`.

### Security

- **OAuth authorize_url scheme/host allowlist** — `Login.svelte`'s
  `signInWithGitHub` now parses the backend-returned `authorize_url`
  with `new URL(...)` and rejects anything that isn't `https:` or
  isn't on a hostname allowlist (currently `['github.com']`). Defends
  the SPA against a misconfigured backend (or future provider plugin)
  that returns a `javascript:` URI or an off-allowlist host. Closes
  `gate-security-authorize-url-no-scheme-host-validation`.
- **401 handler scoped to auth-domain failures only** — the global
  `unauthorizedMiddleware` in `frontend/src/lib/api/client.ts`
  previously called `auth.signOut()` on any 401, which would silently
  sign users out on a per-resource authorization failure (e.g. a stale
  per-org scope). It now reads the typed error envelope from a
  `response.clone()` and only invokes signOut when `error` starts with
  `auth.` (prefix-match, so future `auth.*` codes route through
  automatically). Opaque 401s fail open — surface to the caller. Closes
  `gate-security-401-blanket-signout`.

### Fixes

- **`receive-pack` report-status sideband framing** — when streaming the
  receive-pack reply over the git smart-HTTP transport, the report-status
  packet was double-wrapped on the sideband channel for some clients.
  Hook now writes single-framed. Closes
  `bug-receive-pack-report-status-sideband-wrapping`.

### Refactor

- **Unified `RefUpdate` type across pre-receive and post-receive hooks**
  — the same shape was defined twice with slightly different field
  names. Now lives in one place; both hook handlers import the single
  definition. Pure refactor, no behavior change. Closes
  `refactor-unify-refupdate-across-prereceive-postreceive`.

### Tests

10 coverage gaps surfaced by `gate-tests` and drained as stories. Most
add a single test pinning a behavior the parent feature's spec named
but didn't enforce. One (`gate-tests-oauthcallback-loadme-rejection`)
also fixed the underlying contract violation it surfaced — wraps
`await auth.loadCurrentUser()` in its own try/catch inside
`OAuthCallback.svelte` so a rejected `/api/me` doesn't block the
post-exchange navigate. Items: `gate-tests-router-root-route-home`,
`gate-tests-signout-resets-loadingme`,
`gate-tests-app-authed-on-login-redirect`,
`gate-tests-app-bootstrap-effect`,
`gate-tests-org-row-preventdefault`,
`gate-tests-oauthcallback-loadme-rejection`,
`gate-tests-addorg-reactivity`,
`gate-tests-loadcurrentuser-null-token-noop`,
`gate-tests-picker-submit-name-trim`,
`gate-tests-unknown-role-titlecase`. Frontend test count: 465 → 476
across the cycle.

### Internal

- **Cruft cleanup** (6 items) — dead mock fields, unused `$state` wraps,
  unobserved `vi.spyOn` scaffolding, stale forward-reference comments,
  redundant test setup. `gate-cruft-*` series.
- **Pattern extraction** — 6 new pattern skills captured under
  `.claude/skills/patterns/` covering the Svelte 5 rune-store wrapper,
  snippet-children component shape, openapi-fetch middleware client,
  openapi-fetch result branching, same-origin return-to guard, and the
  jsdom `window.location` defineProperty stubbing pattern. Tracking
  item: `gate-patterns-v0.3.0`.
- **Foundation-doc drift fixes** — `docs/UX.md` updated to describe
  the new home-landing surface; openapi-fetch middleware pattern
  citation added to the patterns index. `gate-docs-*`.
- **Gitignored `.claude/scheduled_tasks.json` lock** — the session-local
  cron lock file no longer dirties `git status`. Closes
  `infra-claude-scheduled-tasks-lock-should-be-gitignored`.

### Deferred to backlog

Three security findings surfaced by `gate-security` were classified as
feature-scope work (cross-stack: frontend + backend coordination
required) rather than single-stride stories. Their `release_binding`
was cleared and they were moved to `.work/backlog/` for proper scoping
in a future release:

- `gate-security-refresh-token-localstorage-exposure` (Medium) — needs
  HttpOnly cookie or Backend-for-Frontend pattern.
- `gate-security-signout-no-backend-revoke` (Low) — needs new
  `POST /api/auth/logout` endpoint with refresh-token revocation.
- `gate-security-oauth-state-no-client-binding` (Low) — needs frontend
  correlation-id storage + backend echo through the OAuth `state`.

## v0.2.0

Released 2026-05-19. GitHub OAuth sign-in works end-to-end for the first
time. The flow was broken at two separate hops in v0.1.x — both 404s
that blocked the round-trip are now closed — and the Login screen's
OAuth button is hardened against double-submit and network failures.

### Features

- **OAuth sign-in actually completes** — the SPA now owns the
  `/auth/oauth/callback` route. After GitHub redirects the browser
  back, the SPA reads `code`+`state` from the query, POSTs them to
  `POST /api/auth/oauth/callback`, stores the returned token pair, and
  navigates the user into the app (honoring `?return_to=` if set).
  Mirrors the existing magic-link exchange pattern at
  `frontend/src/lib/screens/MagicLinkExchange.svelte`. Provider and
  return-to survive the GitHub round-trip via sessionStorage, written
  by `Login.svelte#signInWithGitHub` before the redirect and consumed
  + cleared by the new `OAuthCallback.svelte` on mount.

### Fixes

- **Login GitHub button hits the right route** — `Login.svelte` was
  doing a top-level `window.location.assign('/api/auth/oauth/github/start')`
  to a path that doesn't exist in the backend. The OpenAPI contract
  is `POST /api/auth/oauth/start` with `{provider:"github"}` →
  `{authorize_url}`; the button now uses the shared openapi-fetch
  client and navigates the browser to the returned URL. Closes
  `bug-frontend-oauth-start-route-mismatch`.
- **OAuth flow completes end-to-end** — added the missing SPA
  callback screen + route + auth-gate exclusion + dispatch branch.
  Pre-v0.2.0, even after fixing the start-hop 404, GitHub's redirect
  back to `/auth/oauth/callback` fell through the router to
  `NotFound` and the token exchange never happened. Closes
  `bug-frontend-oauth-callback-handler-missing`.
- **Login GitHub button is double-submit-safe and network-failure-safe** —
  rapid clicks no longer mint and orphan extra `oauth_state` nonces
  (the button is disabled while a start call is in flight), and a
  `fetch` throw (offline, CORS, DNS) routes to the existing error UI
  instead of leaking an unhandled promise rejection. Removed an
  inaccurate "authenticated SPA call" comment that misdescribed the
  endpoint's auth requirement. Closes
  `polish-login-oauth-start-defensive-handling`.

### Internal

- **`scripts/release-bump.sh` preserves file modes** —
  `sed_inplace` now captures the source file's mode with `stat` and
  applies it to the temp file before the `mv`. Pre-v0.2.0 the default
  umask on the temp file stripped the executable bit off
  `bin/jamsesh`, forcing a force-push + retag dance on every release.
  Portable across Linux (`stat -c`) and macOS (`stat -f`). Closes
  `bug-release-bump-sed-inplace-strips-exec-bit`.

### Known issues

- **v0.1.2 has no changelog entry** — the gap between v0.1.1 and this
  release covers two intermediate tagged releases that were never
  logged. The git tag and `release-prep` commit for v0.1.2 are
  present in history; a backfill belongs in a separate doc pass, not
  bundled into a release.
- **`bin/jamsesh` regression harness** — still tracked as
  `testing-bin-jamsesh-regression-harness` (unchanged from v0.1.1).

## v0.1.1

Released 2026-05-19. Operator-experience release: self-host quickstart
template, wrapper-script plugin distribution, OAuth-only deployments now
work, and the e2e quickstart workflow goes green again.

### Features

- **Self-host quickstart template** — `deploy/compose/` ships a turn-key
  `docker-compose.yml` + `.env.example` + `Caddyfile` + `README.md` bundle for
  single-node operators. SQLite default, Postgres opt-in via `--profile
  postgres`. Caddy auto-LE TLS sidecar. Operator workflow: clone → edit two
  env vars → `docker compose up -d`. Documented in `docs/SELF_HOST.md` §1.0
  as the recommended starting point.
- **Wrapper-script plugin distribution** — `bin/jamsesh` is now a bash
  wrapper that downloads the matching per-arch portal-client binary from the
  release's GitHub assets on first invocation, verifies sha256 against the
  signed `checksums.txt`, optionally validates the Sigstore cosign bundle
  when `cosign` is on PATH, caches at `${CLAUDE_PLUGIN_DATA}/bin/`, and
  execs. Same pattern as `gh extension install`. The previous mirror-repo
  pattern (`<owner>/jamsesh-cc-plugin`) is retired — `release.yml` no
  longer publishes to a separate marketplace repo; the Claude Code plugin
  installs directly from `nklisch/jamsesh`.
- **`deploy/compose/.env.example` `JAMSESH_VERSION` pin** — pinned to `v0.1.1`
  for reproducible operator deploys. Bumped per release.

### Fixes

- **Portal starts cleanly without email configured** — `senders.New` no
  longer hard-fails at init when `email.from` is empty. `Provider == ""`
  triggers the new `disabledSender` (returns `ErrMagicLinkNotEnabled` on
  send), letting OAuth-only and no-auth deployments boot. Magic-link
  requests against a portal without email configured return
  `400 auth.magic_link_not_enabled`. Invite emails skip silently when the
  sender is disabled; the invite is still created and returned for the
  host to share manually.
- **`/readyz` storage check passes from a fresh install** — portal
  `MkdirAll(cfg.Storage)` (best-effort) at startup so the readiness probe
  doesn't return 503 until the first push lazily creates session-repo
  parent dirs. Logs and continues on permission denied so it doesn't mask
  other fail-fast paths.
- **`<owner>` placeholder replaced with `nklisch`** across `docs/`,
  `README.md`, `deploy/compose/`, `Caddyfile`. The previous placeholder
  meant a fresh-clone operator setup would deploy a non-existent
  `ghcr.io/<owner>/jamsesh` image; now the default Just Works.
- **Quickstart CI workflow green** — `JAMSESH_EMAIL_FROM` added to the
  workflow env (it was masked by the email-init issue above; the
  workflow-side workaround is the canary that lets us drop the env var
  once a future release removes the underlying require).
- **Clustered mode: git smart-HTTP hydration** — git operations on a pod
  that didn't handle the session-create call now hydrate the bare repo
  from object storage via `LifecycleManager.AcquireForRequest` before
  serving. Previously all peer-pod git operations returned 500. This
  closes the largest cluster of e2e failures from v0.1.0.
- **CI release pipeline** — `release.yml` `marketplace:` job deleted; new
  version-assertion step in `sign-and-release` fails the release fast if
  `bin/jamsesh`'s `JAMSESH_PLUGIN_VERSION` constant doesn't match the
  pushed tag. The wrapper binary's pinned version is now part of the
  release contract.

### Internal

- Release process updated in `docs/RELEASING.md`: "Cutting a release"
  steps 1–6 now include both the compose-template and wrapper-binary
  version bumps; deleted the "One-time bootstrap: marketplace plugin
  repo" section entirely (no longer applicable).
- `docs/SECURITY.md` §"Supply chain and integrity" reworded to describe
  GitHub-release-asset distribution + wrapper-time `bin/jamsesh`
  verification instead of the retired marketplace-repo flow.

### Known issues

- **Clustered-mode e2e tests** (chaos, fuzz, several failure-mode and
  golden tests) remain red. Single-mode is unaffected. Tracked as
  follow-ups: parsed in this session's substrate as
  `bug-receive-pack-report-status-sideband-wrapping` (concrete protocol
  fix for `TestObjectStorageRPO0`-class failures) plus broader
  clustered-mode lease-on-API and fixture-timing work. Self-host operators
  running single-mode (the documented default) see no impact.
- **`bin/jamsesh` regression-test harness** — the wrapper has no
  automated test suite yet. Tracked as
  `testing-bin-jamsesh-regression-harness`.

## v0.1.0

Released 2026-05-18. Initial release.

jamsesh is real-time collaborative AI pair-programming via Claude Code. v0.1.0
ships the full foundation: a Go portal server, a Claude Code plugin, a Svelte
SPA frontend, and supporting deploy/distribution infrastructure.

### Features

- **Portal foundation** — multi-tenant data layer (orgs, accounts, sessions,
  members) with `org_id` enforcement through sqlc-generated queries, TLS-aware
  HTTP skeleton with chi router and shared middleware, OpenAPI-driven REST
  bootstrap, refresh/revoke token flows, GitHub OAuth and magic-link auth with
  pluggable email senders, account/org provisioning, and invite flows.
- **Git smart-HTTP** — `git-upload-pack` / `git-receive-pack` over HTTP with
  bearer auth, bare-repo storage helpers, archive endpoints, pre-receive
  validators (ref naming, size limits, commit metadata, trailer enforcement),
  and post-receive event emission.
- **Auto-merger** — three-way merge engine with go-git, safe-auto-resolve
  semantics for trailer-only conflicts, outcome application back to the bare
  repo, and a worker subscriber driven by the events log.
- **Portal API** — sessions REST (lifecycle, listing, state digest, ref
  actions, invites, member removal), comments REST, events log with OpenAPI
  envelopes, MCP endpoint with `post_comment` / `resolve_comment` / `fork` /
  `query_session_state` tools, and a WebSocket gateway with fanout.
- **Claude Code plugin** — local `jamsesh` binary with browser/device-code
  OAuth, refresh-aware portal client, router state + MCP wiring, fetch/push/
  stop hooks with a retry queue, session slash-commands
  (`join`, `status`, `fork`, `mode`), and packaging with a teaching skill.
- **Portal UI** — Svelte 5 SPA foundation (Vite, routing, login, chrome,
  API/WS token plumbing), design system (tokens, components, fixed test
  fixtures), session list, session view shell with ref tree, artifact pane,
  comment composer, and ref actions (menu, dialogs).
- **Finalize flow** — plan generation (locks schema + REST, fetch token,
  plan fetch + script, mark-shipped semantics), plugin `finalize` /
  `finalize-run` commands with source selection and cleanup, portal curation
  view (screen + route, squash editor, co-author chips).
- **Cloud-native deploy** — routing layer (consistent-hash core, k8s
  discovery, hint cache, MCP header propagation, service wiring, metrics +
  docs), hydration handoff (hydrator, lifecycle, wiring), Postgres-backed
  lease fencing (schema, interface + no-op, Postgres implementation, factory
  + retention), object-storage sync (manifest, pipeline, backend, provider
  extensions, wiring), operational polish (DB pool + lock, `readyz`, metrics,
  secrets-from-file, graceful shutdown, docs).
- **E2E test infrastructure** — module skeleton, Playwright bootstrap,
  Testcontainers fixtures, portal image build, OAuth base-URL configuration,
  CC driver, and CI workflow.
- **E2E test coverage** — golden-path (onboarding/auth, session lifecycle,
  collaborative merge, finalize, CC driver env fix), failure-mode (REST
  validation, config + deps, interrupted ops, SPA error states), chaos
  (network + provider, runtime + clock), and fuzzing (MCP tool input,
  pre-receive validators).
- **E2E CND coverage** — cluster fixture (PortalCluster, MinIO, router image,
  smoke), routing layer (consistent-hash, hint cache, MCP header, k8s
  discovery, 503-retry, backend-dead, pod-disappears chaos), hydration
  handoff (infra, golden, lifecycle, failure, chaos), lease fencing (infra,
  golden, failure, chaos, fuzz), object-storage sync (failure-startup,
  failure-write-rejected, chaos-partition, fuzz-dsn, fuzz-manifest), and
  operational polish (`readyz`, file-secrets, metrics, shutdown deadline,
  migration lock).
- **Distribution** — multi-arch release build pipeline (Linux/macOS/Windows,
  amd64/arm64) with cosign keyless signing, SBOM generation, SLSA build
  provenance, and checksum signing; Docker image build with multi-arch push
  and cosign signing; marketplace publish workflow that mirrors the plugin
  and per-arch binaries; self-host docs (README, SELF_HOST, quickstart CI).

### Security

Gate-security findings hardened before ship:

- Reject GitHub OAuth logins with unverified primary email.
- Validate `fork` ref names against path-traversal in the MCP tool.
- Enforce REST body-size limits and per-route caps.
- Rate-limit auth endpoints (magic-link request, OAuth exchange, refresh).
- Stream `receive-pack` request bodies instead of buffering.
- Require bearer-account match on revoke-token.
- Add security-headers middleware to every response.
- Restrict SQLite default DSN file mode.
- Move WebSocket bearer tokens off the URL onto a ticket exchange and redact
  any residual token-in-URL paths from logs.
- Sanitize HTML rendered from WebSocket events (XSS hardening on
  ActivityFeed).
- Redact tokens in verbose `git` logs and debug logs.
- Shell-escape finalize-script arguments and reject `..` in target branch.
- Move magic-link tokens off the URL into a POST body.
- Authenticate the metrics endpoint.

### Tests

Gate-tests coverage gaps closed before ship — coverage now spans
acceptance-criteria assertions for: ActivityFeed XSS across all event types,
auto-merger apply commit format, safe-resolve skip semantics, finalize lock
concurrent overrides + shell escape, GitHub OAuth unverified email, hint
cache LRU under concurrency, hydration failure unskip, MCP fork ref
traversal, MCP fuzz seed corpus, metrics endpoint auth, object-storage
write-rejected unskip, Postgres lease CI wiring, rate-limit auth,
receive-pack concurrent semaphore, REST body-size cap, revoke-token
cross-account, ring rebalance cardinality, router discovery shutdown, S3
probe failure modes, security headers, stale fencing token unskip, WS bearer
redact, WS client cursor replay fixture.

### Cruft

Gate-cruft cleanups before ship — removed dead code, exported test shims,
and stale compatibility surface: `disableSSL` config, `isPermanentCode`,
`realClock`, `stubs.go`, `timefmt.go`, `withOpenURL`, frontend unused
imports/params, and wired-or-deleted gates for `lifecycle-release`,
`refresher`, `router-kube-discovery`, `automerger-exported-test-shim`,
`buildinfo-string`, `test-only-exports-cluster`.

### Documentation

Gate-docs drift fixes before ship — foundation docs now match
implementation across: architecture (k8s discovery, unscoped routes,
`git-http-backend` removed), protocol (OpenAPI version, WS envelope version,
unscoped routes, missing endpoints), self-host (OAuth callback URL, K8s env
vars, portal URL default, email and OAuth future-release notes), UX
(non-existent slash commands removed), and the `openapi-typescript` repo
skill (version pins refreshed).

### Internal

- 8 reusable code patterns extracted into `.claude/skills/patterns/` with an
  index in `.claude/rules/patterns.md` for future feature work to inherit.
- 4 Low-severity gate findings and 1 product bug deferred to backlog with
  audit trail via `gate_origin` / item bodies.
