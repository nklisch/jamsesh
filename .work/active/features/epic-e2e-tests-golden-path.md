---
id: epic-e2e-tests-golden-path
kind: feature
stage: drafting
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
