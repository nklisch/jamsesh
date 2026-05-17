---
id: epic-e2e-tests-infrastructure-testcontainers-fixtures
kind: story
stage: implementing
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
