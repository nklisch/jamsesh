---
id: epic-e2e-tests-infrastructure
kind: feature
stage: drafting
tags: [e2e-test, testing]
parent: epic-e2e-tests
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Tests — Infrastructure

## Brief

The foundation feature. Builds the scaffolding every other e2e feature
consumes: the test module layout, container fixtures, the Claude Code
hook-lifecycle driver, the Playwright bootstrap, the build pipeline, and
the CI integration. This feature lands a single trivial green spec
(`portal /healthz returns 200 through the Testcontainers stack`) that
proves the scaffolding works end-to-end before any content layer starts.

## Scope

- `tests/e2e/` Go module (separate `go.mod` from project root) with
  shared fixture packages.
- Testcontainers-Go integration for spinning per-test Postgres, MailHog,
  WireMock, mock-oauth2-server, Toxiproxy. The portal-under-test is run
  from a deterministic image built by `make go-build` then `docker build
  -f Dockerfile.test`.
- `tests/e2e/fixtures/ccdriver/` — Go package simulating Claude Code's
  hook event sequence by invoking `jamsesh hook <subcommand>`
  subprocesses with crafted JSON stdin. Exposes a `Driver` type with
  methods like `StartSession()`, `SubmitPrompt()`, `Stop()`. Wraps
  fixture filesystem for `${CLAUDE_PLUGIN_DATA}`.
- `tests/e2e/playwright/` — Playwright config (TypeScript), Chromium
  headless, fixtures wiring against the portal URL exported by the Go
  layer.
- `docker-compose.test.yml` at repo root as a developer-facing escape
  hatch for manual reproduction.
- `make test-e2e` target invoking Go suite then Playwright suite.
- `.github/workflows/e2e.yml` running `make test-e2e` on PRs.
- Documentation in `docs/SELF_HOST.md` and `docs/research/` updated to
  reference the e2e setup if drift is found.

## Out of scope

No content tests (golden-path, failure-mode, chaos, fuzzing) — each
lands in its own feature.

## Foundation references

- `docs/ARCHITECTURE.md` — Portal subcomponents (what we boot in
  containers)
- `docs/SPEC.md` — Stack and contract generation (binary build path,
  embedded SPA)
- `docs/SELF_HOST.md` — Deployment shape (informs the test image
  Dockerfile)
- `.work/active/epics/epic-e2e-tests.md` — Parent epic mock policy
  (constrains every choice here)

## Acceptance criteria

- [ ] `cd tests/e2e && go test ./scaffolding/...` brings up the full
      docker-compose stack via Testcontainers, asserts `GET /healthz` →
      200, and tears down cleanly
- [ ] `tests/e2e/fixtures/ccdriver/` exposes a `Driver` type plus a
      contract test against a frozen JSON payload set
- [ ] `tests/e2e/playwright/smoke.spec.ts` opens `/` in headless
      Chromium and asserts the login screen renders
- [ ] `make test-e2e` runs the Go suite then the Playwright suite with
      a single command
- [ ] `.github/workflows/e2e.yml` runs `make test-e2e` on every PR and
      blocks merge on failure
- [ ] Build is reproducible — same binary SHA from the same git SHA
      (matters for the portal image used in tests)
- [ ] Suite tearDown leaves no dangling containers or test volumes on
      a clean exit OR a failure (verified by a CI cleanup-audit step)
