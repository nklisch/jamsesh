---
id: epic-e2e-tests-infrastructure-playwright-bootstrap
kind: story
stage: done
tags: [e2e-test, testing, ui]
parent: epic-e2e-tests-infrastructure
depends_on: [epic-e2e-tests-infrastructure-testcontainers-fixtures]
release_binding: v0.1.0
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

## Implementation notes

### Files created

- `tests/e2e/playwright/package.json` — `@playwright/test` pinned at `1.45.0`
  (matches `mcr.microsoft.com/playwright:v1.45.0-jammy`); `@types/node` at
  `20.19.41`; `typescript` at `5.4.5`; all exact versions, no `^` prefix
- `tests/e2e/playwright/playwright.config.ts` — reads `PORTAL_URL` env var
  (default `http://localhost:8443`); `trace: "retain-on-failure"`; HTML
  reporter outputs to `playwright-report/`; retries 1 in CI, 0 locally
- `tests/e2e/playwright/tsconfig.json` — `strict: true`,
  `noUncheckedIndexedAccess: true`, `moduleResolution: "Bundler"`,
  `target: "ES2022"`; `npx tsc --noEmit` passes clean
- `tests/e2e/playwright/smoke.spec.ts` — two tests (see selector notes below)
- `tests/e2e/playwright/README.md` — URL handoff docs, local run instructions,
  trace viewer instructions
- `tests/e2e/playwright/.gitignore` — excludes `node_modules/`,
  `playwright-report/`, `test-results/`

### Makefile change

`test-e2e-playwright` target now runs `npm install --silent` then
`npx playwright install --with-deps chromium` (idempotent) then
`npx playwright test` when `tests/e2e/playwright/` exists.

### Selector chosen

Inspected `frontend/src/lib/screens/Login.svelte` and
`frontend/src/lib/components/Input.svelte`. The magic-link form renders:

```html
<input type="email" placeholder="you@example.com" />
```

No `data-testid` attributes are present. The selector used is:

```typescript
page.getByPlaceholder("you@example.com")
```

This is a semantic handle — it fails only on a deliberate copy change and will
not silently pass if the magic-link form disappears.

### Router behaviour

`App.svelte` has an auth guard: `$effect` redirects unauthenticated visitors
from any route to `/login`. Navigating to `/` in a fresh browser session
triggers this redirect, so the smoke spec's `page.goto("/")` reliably lands
on the login screen. The second test navigates to `/login` directly as an
additional verification.

### Verification

- `npx tsc --noEmit` — exits 0, no type errors
- `npx playwright test --list` — lists 2 tests in `smoke.spec.ts`
- Trace artifacts confirmed: failed run produced
  `test-results/smoke-*/trace.zip` as expected
- Failure mode without installed browsers: "Executable doesn't exist" (browser
  not installed) — infrastructure problem, not a TS/config error; the
  `make test-e2e-playwright` target installs browsers before running

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Commit incidentally bundled `.mockups/screens/org-session-invite-policy-settings/*.html` from an unrelated scope (~1000 lines of HTML). Likely picked up by a PostToolUse staging hook on untracked files. Inert side effect.
- `playwright.config.ts > retries` / `workers` use `process.env["CI"] ? ... : ...` — relies on empty-string-is-falsy. Explicit `process.env["CI"] != null` would be more robust if some CI sets `CI=false`. Minor.

**Notes**: Selector choice (`getByPlaceholder("you@example.com")`) is semantic and well-justified inline in the spec. The two-test split (visibility + interactivity) is the right scope for a smoke spec. Strict TS with `noUncheckedIndexedAccess` is correctly applied throughout. Pinned versions (no `^`) for `@playwright/test` 1.45.0 match the CI image tag.
