---
id: epic-e2e-tests-golden-path-finalize
kind: story
stage: implementing
tags: [e2e-test, testing]
parent: epic-e2e-tests-golden-path
depends_on: [epic-e2e-tests-golden-path-collab-merge]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Golden — Finalize curation + execution

## Scope

Two specs that together prove the finalize flow: a human acquires the
lock, fetches the squash plan, runs the plan against a local checkout,
and produces a single coherent commit on the target branch with all
co-authors in the trailers.

- `tests/e2e/golden/finalize_plan_test.go` (Go) — full finalize REST +
  plan-execution flow
- `tests/e2e/playwright/finalize.spec.ts` (Playwright) — the UI
  curation flow on `/orgs/{orgID}/sessions/{sessionID}/finalize`

## Go spec invariant

After a session reaches 5 commits on `draft`, the finalize-plan flow
(acquire lock → fetch plan → execute plan against a local checkout)
produces a single commit on a target branch where the commit message
contains `Co-authored-by:` trailers for every agent that contributed.

## Playwright spec invariant

The finalize curation UI loads the curation tree, lets the user select
squash mode + edit the message, and clicking "generate plan" returns
a plan body that exactly matches what `GET /finalize-plan` returns
when called from the test process.

## Files to create / modify

- `tests/e2e/golden/finalize_plan_test.go` — the Go spec
- `tests/e2e/playwright/finalize.spec.ts` — the Playwright spec
- `tests/e2e/fixtures/checkout/checkout.go` (NEW) — helper that
  creates a local "source repo" tmpdir, runs the plan body's shell
  sequence against it, and exposes `Log()`, `Branch()`, `CommitMsg()`
  for assertions

## Acceptance criteria

- [ ] Go spec brings up a session, drives it to 5 commits via
      ccdriver (2 agents), acquires the finalize lock via
      `POST /finalize/lock`, fetches the plan via
      `GET /finalize-plan`, runs the plan in a tmpdir, asserts a
      single commit on the target branch with the expected
      co-author trailers
- [ ] Playwright spec navigates to the finalize URL, asserts the
      curation tree renders, selects squash mode, modifies the
      commit message, clicks "generate plan", asserts the
      downloaded plan body matches the REST response
- [ ] Lock state machine exercised: `acquire → patch → release`
      paths verified
- [ ] No fixture talks to real source forges or external git
      remotes

## Notes for the implementer

- The finalize handlers are in `internal/portal/finalize/`. Read
  `handler.go` to see the REST entry points. The plan format is
  documented in `docs/ARCHITECTURE.md > Reconciliation (local)`.
- The fetch-token endpoint (`POST /finalize-plan` returns a
  short-lived token plus the plan body) — read
  `internal/portal/finalize/fetch_token.go`
- The plan body is mode-aware shell sequence. In squash mode it's
  `git fetch <source> ; git checkout -b <branch> <base>; git
  cherry-pick --no-commit <commit-1>...<commit-N>; git commit
  --author=<runner> -F <heredoc>`.
- Use `t.TempDir()` for the "source repo" — initialize a fresh `git
  init` then run the plan body as a shell subprocess; assert the
  resulting `git log` shape
