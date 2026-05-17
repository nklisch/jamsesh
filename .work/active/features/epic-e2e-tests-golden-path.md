---
id: epic-e2e-tests-golden-path
kind: feature
stage: done
tags: [e2e-test, testing]
parent: epic-e2e-tests
depends_on: [epic-e2e-tests-infrastructure]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Tests — Golden Path

## Brief

The 5 user journeys that, if broken, make jamsesh broken. Tests run the
whole stack via the Testcontainers fixtures landed by
`infrastructure`. Every assertion is on a user-visible outcome — HTTP
response shape, WS event payload, file contents on a real bare repo
clone, exit code from a real `jamsesh` subcommand, DOM state in
Playwright. No mock-invocation assertions.

## Scope

### Go specs (5 specs)

1. **`join_and_push.spec`** — Two simulated agents (via `ccdriver`)
   join the same session, push commits on independent refs, observe
   each other's commits via `git fetch`, and see `commit.arrived`
   events in the WS event stream. Invariant: after both agents push,
   each agent's local working copy can fetch the peer's ref tip.

2. **`auto_merge.spec`** — Agent A pushes on a sync ref, Agent B pushes
   a non-conflicting change on a different sync ref, the auto-merger
   advances `draft`, both `merge.succeeded` events land in the WS
   stream, and `git fetch && git log draft` shows both source commits
   reachable. Invariant: non-conflicting pushes converge in draft
   without human intervention.

3. **`fork_and_comment.spec`** — Agent A calls `fork` via MCP from a
   draft commit, lands on `<user>/fork-<sha7>`, posts a comment via MCP
   addressed to `@agent-b`, Agent B's `user-prompt-submit` hook digest
   surfaces the comment. Invariant: addressed comments reach the
   addressee's next-turn context.

4. **`finalize_plan.spec`** — A two-agent session reaches a state where
   `draft` has 5 commits; a human (simulated via direct REST + `jamsesh
   finalize --local`) acquires the finalize lock, fetches the squash
   plan, the plan body runs against a local checkout, and produces a
   single commit on the target branch with all co-authors in the
   trailers. Invariant: the finalize flow produces a single coherent
   commit reflecting all agents' work with attribution.

5. **`org_invite_and_session_invite.spec`** — A user creates an org,
   invites another user (magic-link via MailHog), the invitee accepts,
   the creator invites them to a session, they accept, and they can
   read the session's refs via the smart-HTTP. Invariant: the
   invitation chain ends at a working session-member git clone.

### Playwright specs (3 specs)

1. **`login.spec.ts`** — Open `/login`, request magic link, fetch the
   link from MailHog's HTTP API, navigate to the exchange URL, assert
   landing on `/orgs/<id>/sessions`. Invariant: the magic-link flow
   ends with the user authenticated and on the sessions list.

2. **`session_list.spec.ts`** — Authenticated user opens
   `/orgs/<id>/sessions`, sees their sessions populated from the
   backend, clicks one, lands on the session view shell, and the WS
   subscribes (verified by a backend probe that emits an event and
   asserts the SPA renders it). Invariant: the SPA's WS subscription
   delivers events end-to-end.

3. **`finalize.spec.ts`** — Authenticated user opens a session at
   `/orgs/<id>/sessions/<id>/finalize`, sees the curation tree,
   selects squash mode, edits the message, and clicks "generate plan".
   Asserts the plan body is downloadable and matches the REST
   response. Invariant: the finalize UI generates a plan a human can
   execute locally.

## Out of scope

- Failure-mode coverage (its own feature)
- Chaos / network-failure scenarios (chaos feature)
- Property-based / fuzzing (fuzzing feature)
- Multi-tenancy cross-org leakage tests (covered by unit + sqlc-level
  tests under `internal/db/store/cross_org_test.go`; the e2e gate-tests
  audit will flag if e2e coverage is genuinely required)

## Foundation references

- `docs/openapi.yaml` — REST shapes the Go specs assert against
- `docs/PROTOCOL.md` — MCP tool semantics, WS event types
- `docs/ARCHITECTURE.md > Data flow: a turn` — the journey diagram the
  specs trace
- `.work/active/epics/epic-e2e-tests.md` — parent mock policy

## Acceptance criteria

- [ ] 5 Go specs green, each with one named invariant in its top-level
      doc comment
- [ ] 3 Playwright specs green in headless Chromium
- [ ] Full suite runs in under 5 minutes on CI (golden-path is the hot
      path; failure/chaos/fuzz suites are slower by design)
- [ ] Each spec sets up its own org via the real `POST /orgs` path; no
      direct DB inserts
- [ ] Each spec tears down its containers without manual intervention
      on green AND red runs

## Design decisions

Locked under autopilot (2026-05-17):

- **5 stories, not 8**. The original brief listed 5 Go specs + 3
  Playwright specs (8 total). Grouping them into 4 journey arcs
  (auth, session-lifecycle, collab-merge, finalize) + 1 prereq
  (ccdriver env fix) gives cleaner per-story scope and avoids
  fragmenting tightly-coupled assertions across stories. Each
  journey story owns both the Go spec(s) and any Playwright spec for
  the same arc.

- **Promoted `ccdriver-subprocess-env-inheritance` from backlog**.
  The journey specs all invoke the `jamsesh` binary via ccdriver.
  The known env-inheritance bug must land before any journey story
  can be implemented. Made it the prereq child story
  `epic-e2e-tests-golden-path-ccdriver-env-fix`; all journey stories
  depend on it.

- **Per-test fresh portal container**. Matches the portal fixture's
  existing `Start` pattern. Parallel test runs spin many containers
  — acceptable trade-off for state isolation. The Postgres /
  MailHog / WireMock / Toxiproxy containers stay shared via
  `sync.Once` (already implemented).

- **Pre-built `jamsesh` binary** via a new shared fixture
  `tests/e2e/fixtures/binary/jamsesh.go`. `func Build(ctx, t)
  string` runs `go build ./cmd/jamsesh` once per test binary using
  `sync.Once` and returns the absolute path. Avoids rebuild on every
  spec.

- **Postgres for all golden-path specs**, matching the
  infrastructure smoke spec. Driver-matrix coverage (SQLite parallel
  run) is deferred.

- **Magic-link, not OAuth, for the auth journey**. Parent feature
  design specified MailHog. OAuth flow exercise lives in the
  `failure-mode` feature where the smaller surface is easier to
  drive.

- **Three new shared fixtures**:
  - `tests/e2e/fixtures/binary/jamsesh.go` — pre-built binary
  - `tests/e2e/fixtures/wsclient/wsclient.go` — WebSocket client
    that connects with subprotocol-token auth, exposes a `WaitFor`
    channel for event assertions
  - `tests/e2e/fixtures/gitclient/gitclient.go` — git wrapper that
    produces commits with the required `Jam-*` trailers and pushes
    via smart-HTTP
  - `tests/e2e/fixtures/mcpclient/mcpclient.go` — typed MCP tool
    invocations via the official SDK's streamable-http client
  - `tests/e2e/fixtures/mailhog/messages.go` — `LatestMessageTo`
    helper extending the existing mailhog fixture
  - `tests/e2e/fixtures/checkout/checkout.go` — local source-repo
    sandbox for executing finalize plan bodies

  These are owned by the first journey story that needs them but
  declared here so the implementor knows the shared-fixture layout.

## Story decomposition

5 stories under this feature:

1. `epic-e2e-tests-golden-path-ccdriver-env-fix` — 3-line fix to
   `tests/e2e/fixtures/ccdriver/driver.go` so subprocesses inherit
   host environment (PATH, HOME, etc.). Promoted from backlog. No
   deps.

2. `epic-e2e-tests-golden-path-onboarding-auth` — `onboarding_test.go`
   (Go) + `login.spec.ts` (Playwright). Magic-link flow + invite
   acceptance + org membership. Depends on the env fix.

3. `epic-e2e-tests-golden-path-session-lifecycle` —
   `session_join_and_push_test.go` (Go) + `session_list.spec.ts`
   (Playwright) + the `wsclient` and `gitclient` shared fixtures.
   Two agents joining, pushing, observing peer activity. Depends on
   onboarding (uses authenticated agent setup).

4. `epic-e2e-tests-golden-path-collab-merge` — `auto_merge_test.go` +
   `fork_and_comment_test.go` + the `mcpclient` shared fixture. The
   convergence + MCP-tool-use arc. Depends on session-lifecycle (uses
   the same multi-agent harness).

5. `epic-e2e-tests-golden-path-finalize` — `finalize_plan_test.go` +
   `finalize.spec.ts` + the `checkout` shared fixture. The "human
   ships the work" arc. Depends on collab-merge (needs a session
   with multi-agent draft state to exercise finalize).

## Implementation Order

Wave 1: `ccdriver-env-fix` (no deps).
Wave 2: `onboarding-auth` (depends on env fix).
Wave 3: `session-lifecycle` (depends on onboarding).
Wave 4: `collab-merge` (depends on session-lifecycle).
Wave 5: `finalize` (depends on collab-merge).

Each later wave reuses the fixtures from the prior wave. This
serializes the implementation but keeps shared-fixture authorship
clean — each journey story owns the fixtures it introduces, the next
story extends them.

## Pre-mortem

- **WebSocket subprotocol-token auth is fiddly across implementations**.
  Go's standard `gorilla/websocket` and `coder/websocket` clients
  expect different `Sec-WebSocket-Protocol` handling. The wsclient
  fixture should pin one library (use `coder/websocket` to match the
  server) and test the subprotocol path early.
- **Git smart-HTTP basic-auth with bearer tokens** is unusual — the
  bearer goes in as the HTTP Basic password with an empty username
  (or `jamsesh` placeholder). The gitclient fixture must get this
  right or every push fails with 401.
- **Auto-merger timing**: tests assume `merge.succeeded` fires within
  some short window of the source push. If the auto-merger has a
  startup delay or batches commits, the assertion windows may be too
  tight. The wsclient's `WaitFor` should accept a generous timeout
  (5-10s) initially; tighten later if the suite is reliable.
- **Finalize plan execution is the riskiest spec** — it runs a real
  `git cherry-pick` sequence inside the test process. Plan-body
  errors can leave the sandbox repo in mid-pick state. The checkout
  fixture must idempotently clean up via `git cherry-pick --abort`
  on failure.
- **Playwright timing in CI**. Playwright traces are the canonical
  debug artifact. If a Go-spawned portal container takes longer to
  reach `/healthz` than Playwright's default navigation timeout,
  specs fail spuriously. Use `page.goto({ waitUntil: "load" })` with
  generous timeouts.

Risks documented; no spike unit added (the feature is large but the
risks are well-understood, not exploratory).

## Implementation summary (2026-05-17)

All 5 child stories landed:
- `ccdriver-env-fix` (done)
- `onboarding-auth` (done)
- `session-lifecycle` (done)
- `collab-merge` (review)
- `finalize` (review)

**Production bugs found and fixed during implementation**:
- Dockerfile lacked git (session-lifecycle) — production Dockerfile updated to debian+git; separate Dockerfile.e2e added; reviewed and filed `portal-prod-dockerfile-base-image-review` for ops-team review
- `logging.statusRecorder.Unwrap()` missing (session-lifecycle) — broke WS upgrade; fixed in `internal/portal/logging/logging.go`
- `sessions.status` CHECK constraint missing `'finalizing'` (interrupted-ops from failure-mode, surfaced here too) — added migration `00012`
- `receive-pack` never seeded `draft` for new repos (collab-merge) — auto-merger silently skipped merging; fixed in `internal/portal/githttp/receive_pack.go`

**Test-side fixes applied in session** (per user directive):
- authflow QP-decode for magic-link tokens
- onboarding-test email randomization
- ccdriver env-fix test strengthening

**Coverage delivered**:
- Auth journey (magic-link, org create, invite/accept, membership)
- Session lifecycle (join, push, peer fetch, WS events)
- Collab + auto-merger + MCP tools (fork, post_comment, addressed-to digest delivery)
- Finalize curation + plan execution (squash mode + lock state machine)
- 3 Playwright specs (login, session-list, finalize) + smoke + error-states from failure-mode

**Next**: `/agile-workflow:review epic-e2e-tests-golden-path` once the user is ready.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none new at feature level — all 5 children reviewed individually with findings filed appropriately.

**Notes**: All 5 stories landed. The golden-path feature delivered the headline value of the e2e program — it surfaced 4 real production bugs that would have hit users (logging Unwrap, sessions.status migration, Dockerfile git, receive-pack draft seeding). Each was fixed inline as a prerequisite for the test work, with the larger ops concerns (Dockerfile base-image review) filed as backlog for ops review. The 5 journey arcs (auth, session lifecycle, collab+auto-merger+MCP, fork+addressed-comments, finalize+plan-execution) collectively prove jamsesh's value proposition end-to-end.
