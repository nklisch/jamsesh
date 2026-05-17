# Playwright E2E Tests

Browser automation tests for the jamsesh portal SPA, driven by Playwright
against a live portal instance provisioned by the Go e2e layer.

## Prerequisites

- Node 20+
- Docker running locally (`docker info` must succeed)
- Portal e2e image built: `make test-portal-image` from the repo root

## URL handoff from the Go layer

The portal's address is passed via the `PORTAL_URL` environment variable.

**Pattern:**
1. The Go test fixture (`tests/e2e/fixtures/portal`) starts the portal
   container and exposes `.URL` (e.g. `http://localhost:39281`).
2. The Go test sets `PORTAL_URL` in the child process environment before
   invoking `npx playwright test`.
3. `playwright.config.ts` reads `process.env.PORTAL_URL` and sets it as
   `baseURL` for all tests.

For the current bootstrap phase, the developer sets `PORTAL_URL` manually
(see local run instructions below). Go-orchestrated invocation via a single
`make test-e2e` entry point lands in the `ci-workflow` story.

## Running locally

### One-time setup

```bash
# Build the portal e2e image (re-run after changes to the portal binary)
make test-portal-image

# Install Node dependencies and Playwright browsers
cd tests/e2e/playwright
npm install
npx playwright install --with-deps chromium
```

### Confirm the Go fixture layer works

```bash
cd tests/e2e
go test ./scaffolding/ -run TestPortalHealthz -v
```

### Start the portal (standalone)

```bash
docker run --rm -d --name portal-smoke \
  -p 39281:8443 \
  -e JAMSESH_DB_DRIVER=sqlite \
  -e JAMSESH_DB_DSN=:memory: \
  -e JAMSESH_TLS_MODE=behind_proxy \
  -e JAMSESH_EMAIL_FROM=noreply@example.com \
  jamsesh/portal:e2e
```

### Run Playwright tests

```bash
cd tests/e2e/playwright
PORTAL_URL=http://localhost:39281 npx playwright test
```

Or from the repo root:

```bash
PORTAL_URL=http://localhost:39281 make test-e2e-playwright
```

### Stop the portal

```bash
docker stop portal-smoke
```

## Running via make (repo root)

```bash
# Full e2e suite (Go then Playwright)
PORTAL_URL=http://localhost:39281 make test-e2e

# Playwright only
PORTAL_URL=http://localhost:39281 make test-e2e-playwright
```

The `make test-e2e-playwright` target installs npm dependencies and Playwright
browsers on first run (idempotent — subsequent runs skip the install step).

## Running against a live portal

Export the portal address and run normally:

```bash
export PORTAL_URL=https://portal.example.com
cd tests/e2e/playwright && npx playwright test
```

Trace artifacts are produced on failure at `playwright-report/`.

## Viewing traces

When a test fails, Playwright captures a trace archive:

```bash
cd tests/e2e/playwright
npx playwright show-trace playwright-report/<run-id>/trace.zip
```

Or open `playwright-report/index.html` in a browser for the full HTML report.

## Running headed (visible browser)

```bash
cd tests/e2e/playwright
PORTAL_URL=http://localhost:39281 npx playwright test --headed
```

## Test structure

| File | Purpose |
|------|---------|
| `smoke.spec.ts` | Proof-of-life: portal loads, SPA boots, login screen renders |
| `playwright.config.ts` | Playwright config — base URL, retries, trace settings |
| `tsconfig.json` | TypeScript config (`strict: true`, `noUncheckedIndexedAccess: true`) |

## Related items

- Go fixture layer: `tests/e2e/fixtures/portal/`
- Go smoke spec: `tests/e2e/scaffolding/` (`TestPortalHealthz`)
- Feature item: `.work/active/features/epic-e2e-tests-infrastructure.md`
