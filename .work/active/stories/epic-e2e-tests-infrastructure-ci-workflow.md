---
id: epic-e2e-tests-infrastructure-ci-workflow
kind: story
stage: implementing
tags: [e2e-test, testing, infra]
parent: epic-e2e-tests-infrastructure
depends_on: [epic-e2e-tests-infrastructure-testcontainers-fixtures, epic-e2e-tests-infrastructure-ccdriver, epic-e2e-tests-infrastructure-playwright-bootstrap]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Infra — CI workflow

## Scope

GitHub Actions workflow `.github/workflows/e2e.yml` that runs the
full e2e suite (`make test-e2e`) on every PR and push to main.
Keep `quickstart.yml` alongside (different purpose).

## Files to create / modify

- `.github/workflows/e2e.yml` — new workflow
- `docs/SELF_HOST.md` (or wherever CI is documented) — mention the
  new workflow as the canonical e2e gate

## Workflow shape

```yaml
name: e2e
on:
  pull_request:
    branches: [main]
  push:
    branches: [main]
jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
      - name: build portal test image
        run: make test-portal-image
      - name: install playwright browsers
        run: cd tests/e2e/playwright && npm install && npx playwright install --with-deps chromium
      - name: run e2e
        run: make test-e2e
      - name: upload playwright trace on failure
        if: failure()
        uses: actions/upload-artifact@v4
        with:
          name: playwright-trace
          path: tests/e2e/playwright/playwright-report/
```

## Acceptance criteria

- [ ] Workflow runs on PR and push to main
- [ ] Workflow fails when any e2e spec fails (verified by an
      intentional-regression PR that gets reverted)
- [ ] Workflow uploads the Playwright trace on failure for debugging
- [ ] Total runtime under 10 minutes on `ubuntu-latest`
- [ ] `quickstart.yml` continues to pass alongside `e2e.yml` —
      both workflows run on each PR

## Notes for the implementer

- `ubuntu-latest` has Docker preinstalled; Testcontainers-Go uses
  it directly. No `services:` declarations needed
- Cache the Go module cache and `node_modules` to speed up cold
  runs (use `actions/cache@v4`)
- The Playwright image dependency is heavy (~1GB) — the
  `--with-deps` flag fetches it on demand. Consider caching the
  Playwright browsers under `~/.cache/ms-playwright`
- Don't add `--no-verify` or similar bypasses; if the suite is
  flaky, fix the suite, not the workflow
