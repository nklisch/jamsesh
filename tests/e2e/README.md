# E2E Test Suite

End-to-end tests for jamsesh. These tests run the application against real
infrastructure (database, OIDC provider, etc.) spun up via Testcontainers-Go.

## Prerequisites

- **Go 1.26 or newer.** Both the root module (`go.mod`) and this module
  (`tests/e2e/go.mod`) require Go 1.26. CI pins `go-version: '1.26.x'`
  explicitly (in `.github/workflows/e2e.yml` and `release.yml`) rather
  than floating on `stable`, so local builds and CI builds use the same
  toolchain.
- **Docker** running locally (`docker info` must succeed).
- **Portal e2e image** built: `make test-portal-image`.
  - This compiles a static Linux binary and packages it into the unified
    production image (alpine:3.21 + git + ca-certificates), tagged
    `jamsesh/portal:e2e`.
  - Re-run after any change to the portal binary or its Dockerfile.

## How to run

### Fixture self-tests (each fixture in isolation)

```bash
cd tests/e2e && go test ./fixtures/... -v
```

Each fixture package has a self-test that verifies Start succeeds, the
service is reachable, and Cleanup tears down the container.

### Smoke spec (full stack proof-of-life)

```bash
cd tests/e2e && go test ./scaffolding/ -run TestPortalHealthz -v
```

`TestPortalHealthz` spins up the full stack — Postgres, MailHog, WireMock,
Toxiproxy, and the portal — and asserts `GET /healthz` returns 200. This is
the keystone test: if it passes, the e2e foundation is working.

### All e2e Go tests

```bash
cd tests/e2e && go test ./...
```

Tests skip cleanly when Docker is unavailable or the portal image has not
been built (no failure noise in CI without Docker).

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

## Fixture packages

All fixtures live under `tests/e2e/fixtures/`:

| Package       | Container image                      | Exposes                                    |
|---------------|--------------------------------------|--------------------------------------------|
| `postgres`    | `postgres:16-alpine`                 | `.DSN`, `.ContainerDSN`, `.Host`, `.Port`  |
| `mailhog`     | `mailhog/mailhog:v1.0.1`             | `.SMTPHost/Port`, `.ContainerSMTPHost/Port`, `.HTTPURL` |
| `wiremock`    | `wiremock/wiremock:3.5.4`            | `.URL`, `.ContainerURL`                    |
| `toxiproxy`   | `ghcr.io/shopify/toxiproxy:2.7.0`    | `.AdminURL`                                |
| `portal`      | `jamsesh/portal:e2e`                 | `.URL`                                     |

**Container vs host addresses.** Fixtures expose two sets of addresses:
- Host-side (e.g. `.DSN`, `.SMTPHost`, `.URL`) — use these from the test
  process to inspect or assert state.
- Container-side (e.g. `.ContainerDSN`, `.ContainerSMTPHost`, `.ContainerURL`)
  — use these when configuring the portal fixture to reach other containers
  across the Docker bridge network.

### Postgres per-test isolation

The Postgres fixture shares a single container per test binary (`sync.Once`)
and creates a fresh database per test call (`test_<random>`). Each database
is automatically dropped by `t.Cleanup`. This keeps startup fast while
ensuring test isolation.

## Where containers come from

Test infrastructure is provisioned by Testcontainers-Go fixtures. Each
fixture manages its own container lifecycle; the Go test binary pulls images
at runtime and tears them down after the suite.

No manual `docker compose up` is required — `go test ./...` is the
single entry point.

## Feature item

`.work/active/features/epic-e2e-tests-infrastructure.md`
