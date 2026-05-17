---
id: epic-e2e-tests-infrastructure-testcontainers-fixtures
kind: story
stage: done
tags: [e2e-test, testing]
parent: epic-e2e-tests-infrastructure
depends_on: [epic-e2e-tests-infrastructure-portal-image-build, epic-e2e-tests-infrastructure-portal-oauth-base-url]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Infra — Testcontainers fixtures + healthz smoke spec

## Scope

Implement the Go fixture packages that spin Postgres, MailHog,
WireMock, Toxiproxy, and the portal-under-test image via
Testcontainers-Go. Plus the proof-of-life smoke spec that brings
up the full stack and asserts `GET /healthz` returns 200.

## Files to create / modify

- `tests/e2e/go.mod` — add deps:
  `github.com/testcontainers/testcontainers-go` (latest stable),
  `github.com/testcontainers/testcontainers-go/modules/postgres`,
  the standard `wiremock-go` client (or use generic
  `testcontainers.GenericContainer` with HTTP setup)
- `tests/e2e/fixtures/postgres/postgres.go` — `Start(ctx, t,
  Options) *Postgres` exposing `.DSN`, `.Port`. Uses `sync.Once` to
  share a container per test binary; issues `CREATE DATABASE
  test_<random>` per test for isolation
- `tests/e2e/fixtures/mailhog/mailhog.go` — `Start(ctx, t) *MailHog`
  exposing `.SMTPHost`, `.SMTPPort`, `.HTTPURL` (for inspecting
  captured mail via `/api/v2/messages`). Image: `mailhog/mailhog:v1.0.1`
- `tests/e2e/fixtures/wiremock/wiremock.go` — `Start(ctx, t,
  Mappings) *WireMock` exposing `.URL`. `Mappings` is a map of
  named mapping-JSON file paths mounted into
  `/home/wiremock/mappings/`. Image: `wiremock/wiremock:3.5.4`
- `tests/e2e/fixtures/wiremock/mappings/github.json` — WireMock
  mappings for the three GitHub OAuth endpoints
  (`/login/oauth/access_token`, `/user`, `/user/emails`) returning
  fixture user data
- `tests/e2e/fixtures/toxiproxy/toxiproxy.go` — `Start(ctx, t)
  *Toxiproxy` exposing `.AdminURL` plus helpers for adding /
  removing toxics in front of named upstreams. Image:
  `ghcr.io/shopify/toxiproxy:2.7.0`
- `tests/e2e/fixtures/portal/portal.go` — `Start(ctx, t, Options)
  *Portal` exposing `.URL`. Uses the `jamsesh/portal:e2e` image
  built by `make test-portal-image`. Each invocation creates a
  fresh container with the given env config
- `tests/e2e/scaffolding/healthz_test.go` — the smoke spec
- `tests/e2e/README.md` — update with how to run, prerequisites
  (Docker, `make test-portal-image`)

## Acceptance criteria

- [ ] `cd tests/e2e && go test ./fixtures/...` runs each fixture's
      self-test (Start succeeds, URL is reachable, Cleanup tears
      down)
- [ ] `cd tests/e2e && go test ./scaffolding/` runs the healthz
      smoke spec green: brings up Postgres + MailHog + WireMock +
      Toxiproxy + portal, asserts `GET /healthz` returns 200,
      tears down without leaving dangling containers
- [ ] Postgres fixture uses shared container with per-test DB
      isolation (`sync.Once` pattern; per-test `CREATE DATABASE`)
- [ ] MailHog HTTP API is reachable from the test process for
      inspection
- [ ] WireMock returns the expected stub responses for the
      mapped GitHub endpoints
- [ ] Toxiproxy admin API is reachable; a no-op proxy can be
      created and torn down
- [ ] Portal fixture handles startup failures gracefully — if the
      image is missing, the test fails with a clear "run `make
      test-portal-image` first" message rather than a Docker
      backtrace

## Notes for the implementer

- Testcontainers-Go's wait strategies are critical for stability —
  use `wait.ForListeningPort()` and `wait.ForHTTP("/healthz")` for
  the portal; use the standard postgres-module wait strategy for
  Postgres
- Per-test DB isolation: random suffix (e.g.
  `test_<8-hex-chars>`); `t.Cleanup` drops the DB
- The portal fixture's `Options` should accept enough env config to
  let the smoke spec wire MailHog + WireMock; reference the
  portal's config env-var list in
  `internal/portal/config/config.go`
- Mark expensive tests with a build tag (`//go:build e2e`) if the
  fixture self-tests take more than a few seconds — keeps casual
  `go test` runs cheap
- If the WireMock client library isn't available or is heavy, use
  `testcontainers.GenericContainer` directly with a Dockerfile-free
  mounted-mappings approach

## Implementation notes

### Files created / modified

- `tests/e2e/go.mod` + `go.sum` — added `github.com/testcontainers/testcontainers-go@v0.42.0`, `github.com/testcontainers/testcontainers-go/modules/postgres@v0.42.0`, `github.com/lib/pq@v1.12.3`
- `tests/e2e/fixtures/postgres/postgres.go` — shared container (`sync.Once`), per-test DB with `CREATE DATABASE test_<8hex>`, `t.Cleanup` drops DB; exposes `.DSN` (host-side) and `.ContainerDSN` (Docker bridge IP, for portal fixture)
- `tests/e2e/fixtures/postgres/postgres_test.go` — self-test + isolation test
- `tests/e2e/fixtures/mailhog/mailhog.go` — `GenericContainer` with `wait.ForAll(ForListeningPort, ForHTTP)`; exposes `.SMTPHost/Port` (host-side) and `.ContainerSMTPHost/Port` (Docker bridge)
- `tests/e2e/fixtures/mailhog/mailhog_test.go` — self-test
- `tests/e2e/fixtures/wiremock/wiremock.go` — `GenericContainer` with file mounts; exposes `.URL` (host-side) and `.ContainerURL` (Docker bridge)
- `tests/e2e/fixtures/wiremock/wiremock_test.go` — self-test including stub response verification
- `tests/e2e/fixtures/wiremock/mappings/github.json` — WireMock stubs for `/login/oauth/access_token`, `/user`, `/user/emails`
- `tests/e2e/fixtures/toxiproxy/toxiproxy.go` — `GenericContainer` with `wait.ForHTTP("/proxies")`; exposes `.AdminURL`
- `tests/e2e/fixtures/toxiproxy/toxiproxy_test.go` — self-test
- `tests/e2e/fixtures/portal/portal.go` — `GenericContainer` with `wait.ForHTTP("/healthz")`; `requirePortalImage` skips with actionable message if image absent; `buildEnv` maps `Options` to `JAMSESH_*` env vars
- `tests/e2e/fixtures/portal/portal_test.go` — SQLite in-memory self-test
- `tests/e2e/scaffolding/healthz_test.go` — full-stack smoke spec
- `tests/e2e/README.md` — updated with prerequisites, fixture table, container-vs-host addressing explanation

### Key discovery: Docker bridge networking

The portal runs inside a Docker container, so it cannot reach the host-mapped ports of Postgres, MailHog, or WireMock via `localhost`. All fixtures now expose `ContainerDSN` / `ContainerSMTPHost` / `ContainerURL` fields built from the Docker bridge IP (`c.ContainerIP(ctx)`). The smoke spec uses these container-side addresses when configuring the portal fixture. The host-side addresses remain available for test-process assertions.

### API shape change (testcontainers-go v0.42.0)

`network.Port.Int()` no longer exists. The correct method is `network.Port.Num() uint16`. All fixtures use `int(port.Num())` where an int is needed.

### No build tags used

Fixture self-tests skip cleanly via `requireDocker(t)` when Docker is unavailable. No `//go:build e2e` tag was added — the clean skip behavior makes the tag unnecessary.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none

**Important**:
- Container logs lost on test failure — `t.Cleanup` terminates containers before logs can be inspected; the portal fixture's "check `docker logs <id>`" hint is unreachable. Filed as `e2e-fixtures-capture-container-logs-on-failure` in `.work/backlog/`.
- OAuth fixture footgun: when `OAuthBaseURL` is empty, the portal would call real github.com if any test exercises the OAuth flow (the fixture defaults `CLIENT_ID`/`CLIENT_SECRET` non-empty). Filed as `e2e-portal-fixture-oauth-base-url-default` in `.work/backlog/`.

**Nits**:
- `requireDocker` is duplicated in every fixture package (5 copies). Acceptable per the design's "pick the simpler option" rule, but a future tidy could extract to `tests/e2e/internal/dockerutil/`.
- `randHex` uses `crypto/rand` for a test-id suffix — overkill but harmless.
- `CREATE DATABASE` / `DROP DATABASE` use `fmt.Sprintf` with a string-interpolated database name. Currently safe (`"test_" + randHex(8)` is closed-domain), but the pattern is a footgun if anyone reuses it. Postgres has no parameterised DDL so the pattern is unavoidable here — just document the constraint near the call sites.
- `tests/e2e/go.mod` declares `go 1.26` (root is `go 1.25.7`). The CI workflow uses `go-version: 'stable'` so this works, but worth confirming whether to pin or document the skew. Already noted in the parent feature's implementation summary.

**Notes**: The Docker-bridge-vs-host-port split (`ContainerDSN` / `ContainerURL` / `ContainerSMTPHost`) is a real-world discovery and well-documented in the README. The fixtures honour `t.Cleanup` discipline. The smoke spec asserts on the user-visible HTTP status, not on mock invocations — anti-tautology guardrail respected. The `JAMSESH_EMAIL_SMTP_TLS=none` in the portal fixture is critical for MailHog (which doesn't speak TLS) and well-caught.
