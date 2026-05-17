# E2E Test Suite

End-to-end tests for jamsesh. These tests run the application against real
infrastructure (database, OIDC provider, etc.) spun up via Testcontainers-Go.

## How to run

### Go-based tests

```bash
# From repo root
make test-e2e-go

# Or directly
cd tests/e2e && go test ./...
```

### Playwright tests (browser automation)

```bash
# From repo root
make test-e2e-playwright

# Or directly
cd tests/e2e/playwright && npm test
```

### Full suite

```bash
make test-e2e
```

This runs Go tests first, then Playwright. The Playwright target no-ops
cleanly if `tests/e2e/playwright/` has not been bootstrapped yet.

## Where containers come from

Test infrastructure (Postgres, Dex OIDC) is provisioned by Testcontainers-Go
fixtures defined in `tests/e2e/fixtures/`. Each fixture manages its own
container lifecycle; the Go test binary pulls images at runtime and tears
them down after the suite.

No manual `docker compose up` is required — `go test ./...` is the
single entry point.

## Feature item

`.work/active/features/epic-e2e-tests-infrastructure.md`
