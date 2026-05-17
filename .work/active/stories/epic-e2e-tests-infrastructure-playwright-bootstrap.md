---
id: epic-e2e-tests-infrastructure-playwright-bootstrap
kind: story
stage: implementing
tags: [e2e-test, testing, ui]
parent: epic-e2e-tests-infrastructure
depends_on: [epic-e2e-tests-infrastructure-testcontainers-fixtures]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Infra — Playwright bootstrap + smoke spec

## Scope

Set up `tests/e2e/playwright/` with Playwright config, TypeScript
config, package.json, and a smoke spec that opens `/login` in
headless Chromium against the portal-under-test (URL exported by the
Go layer).

## Files to create / modify

- `tests/e2e/playwright/package.json` — `@playwright/test` pinned;
  scripts: `test`, `test:headed`
- `tests/e2e/playwright/playwright.config.ts` — base URL read from
  `process.env.PORTAL_URL` (default
  `http://localhost:8443`); headless: true; trace: 'retain-on-failure';
  retries: 1 in CI, 0 locally
- `tests/e2e/playwright/tsconfig.json` — strict TS config
- `tests/e2e/playwright/smoke.spec.ts` — opens `/`, asserts a
  recognizable login element renders (e.g., the magic-link email
  input field) within 5 seconds
- `tests/e2e/playwright/README.md` — explains the URL handoff from
  Go layer, how to run locally, how to run against a live portal
- `Makefile` — update `test-e2e-playwright` target to install
  Playwright browsers (`npx playwright install --with-deps
  chromium`) on first run

## Acceptance criteria

- [ ] `cd tests/e2e/playwright && npm install && npx playwright
      test smoke.spec.ts` runs green when `PORTAL_URL` points at a
      running portal
- [ ] `make test-e2e-playwright` runs the suite, installing
      browsers if not already present
- [ ] The smoke spec asserts on a user-visible DOM element on
      `/login` (not a generic `body` element) — pin the assertion
      to something a regression would actually surface
- [ ] Failed runs produce a Playwright trace artifact at
      `playwright-report/`
- [ ] The TS config is `strict: true` and `noUncheckedIndexedAccess:
      true`

## Notes for the implementer

- The handoff pattern from the design: Go fixtures write the
  portal URL somewhere Playwright reads at config-load time.
  Simplest implementation: the smoke spec is invoked as a child of
  a Go test that has the portal running, with `PORTAL_URL` set as
  an env var on the child process. For now, the bootstrap can use
  the standalone form (developer manually exports `PORTAL_URL`) —
  Go-orchestrated invocation lands in the ci-workflow story
- Pin the Playwright version in `package.json` to match the image
  tag used in CI (`mcr.microsoft.com/playwright:v1.45.0-jammy`)
- Use the magic-link email input as the smoke-test assertion target
  — check `frontend/src/lib/auth/` or similar for the actual
  selector / test-id
- Avoid `setTimeout` / `page.waitForTimeout` in the smoke spec —
  always wait on observable state (selector visibility, network
  responses)
