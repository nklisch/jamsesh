---
id: epic-e2e-tests
kind: epic
stage: done
tags: [e2e-test, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# End-to-End Test Program

## Brief

Jamsesh has ~100 Go unit/integration tests that exercise individual packages
with in-process collaborators (in-memory SQLite, real `git` against tmpdirs,
real chi router, real `coder/websocket`). The frontend has 34 vitest files
under jsdom with a hand-rolled `MockWebSocket`. CI runs only a `/healthz`
smoke test — `go test ./...` never executes in the pipeline and the four
critical user journeys (login, join, push-and-merge, finalize) have zero
end-to-end coverage.

This epic seeds a service-level e2e test program that exercises the portal
and the `jamsesh` plugin binary as black-box artifacts against
container-hosted dependencies, plus a Playwright headless browser layer over
the Svelte SPA. Tests assert on user-visible outcomes (HTTP responses, WS
event payloads, file contents, exit codes, browser DOM) — never on mock
invocations. The program is the first thing CI runs after build and gates
release-deploy via gate-tests bundle audits.

## Mock policy

Service-level mocks only. The ladder, in priority order:

1. **Off-the-shelf service mock** — preferred. Catalog used:
   - **Postgres** → Testcontainers `postgres:16` (and SQLite stays
     in-process for the default-driver matrix).
   - **SMTP sender** → MailHog (`mailhog/mailhog`) — captures sent mail,
     exposes HTTP inspection API.
   - **SendGrid / Postmark / Resend HTTP APIs** → WireMock
     (`wiremock/wiremock`) with mounted mappings JSON per provider.
   - **GitHub OAuth** → mock-oauth2-server
     (`ghcr.io/navikt/mock-oauth2-server`) for token issuance + user/email
     endpoints, fronted by WireMock when finer-grained failure injection is
     required.
   - **Network failure injection** → Toxiproxy
     (`ghcr.io/shopify/toxiproxy`) in front of the portal and DB for chaos
     scenarios.
   - **Container lifecycle chaos** → Pumba (`gaiaadm/pumba`) for kill/pause
     scenarios.
   - **Browser** → Playwright Chromium headless (`mcr.microsoft.com/playwright`)
     driving the Svelte SPA served by the real portal.
2. **Custom mock container** — the **Claude Code plugin lifecycle driver**
   is the only custom artifact. A Go test harness invokes the `jamsesh`
   binary's hook subcommands with crafted JSON stdin to simulate Claude
   Code's event sequence (session-start → user-prompt-submit → pre-tool-use
   → post-tool-use → stop → session-end). The harness lives at
   `tests/e2e/fixtures/ccdriver/` and is built as a Go package consumed by
   the spec files. **Language-matched to the project** (Go), no separate
   toolchain.
3. **In-process mock** — disallowed except with explicit written
   justification in the unit's design notes. None expected at bootstrap.

The three existing in-process mocks (frontend `MockWebSocket`,
`senders.Sender` httptest stub, `storage.Service` stub) stay in their
respective unit-test scopes; the e2e program does **not** import them and
instead drives the real backend.

## Foundation references

- `docs/VISION.md` — the user-visible promise (live jam, recoverable, git-native)
- `docs/ARCHITECTURE.md` — Portal subcomponents, the `jamsesh` binary subcommand surface, turn data flow
- `docs/SPEC.md` — stack (Go + Svelte + SQLite/Postgres), generated contracts, success criteria
- `docs/PROTOCOL.md` — REST + MCP + WS event contracts (what tests assert against)
- `docs/SECURITY.md` — auth boundaries (what failure-mode tests target)
- `docs/openapi.yaml` — REST + WS-envelope schemas (the source of truth e2e tests pin to)

## Design decisions

Locked at bootstrap (this pass):

- **Test runner location.** `tests/e2e/` at repo root. Go-based spec files
  using the standard `testing` package with `t.Run` subtests. The runner
  ships in a separate Go module (`tests/e2e/go.mod`) so the e2e deps
  (Testcontainers, WireMock client) don't bleed into the portal's
  go.mod. **Rationale**: matches Go conventions for integration suites
  living alongside the project; keeps `cmd/portal`'s deps lean.

- **Container orchestration.** Testcontainers-Go for spinning containers
  from test code. **Rationale**: per-test isolation, automatic port
  allocation, automatic cleanup. A `docker-compose.test.yml` is kept as a
  developer-facing escape hatch (manual reproduction outside test runner)
  but the CI path runs through Testcontainers.

- **Portal under test.** Built as a binary by `make go-build` and copied
  into a container with a deterministic image tag. The portal is **not**
  imported as a library by the e2e tests — that's the line between
  service-level and in-process. **Rationale**: e2e validates the shipped
  artifact, not a fresh in-process build.

- **Frontend coverage.** Playwright headless Chromium against the real
  portal serving its embedded SPA build. Playwright tests live in
  `tests/e2e/playwright/` and run after the Go e2e gate passes. Three
  golden journeys covered by Playwright (login + session list + finalize)
  + one cross-stack failure (WebSocket reconnect under network drop).
  **Rationale**: rules out the cross-stack gaps the existing vitest +
  MockWebSocket combo can't cover, without buying a full UI test program.

- **Plugin-lifecycle driver.** Custom Go driver under
  `tests/e2e/fixtures/ccdriver/` simulates the Claude Code hook event
  sequence by invoking `jamsesh hook <subcommand>` subprocesses with
  crafted JSON stdin. **Rationale** (matches the upstream tech rule):
  jamsesh is Go, the binary is Go, the hooks consume JSON — a Go driver
  is the most faithful substitute. Real Claude Code is slow,
  nondeterministic, and depends on a remote API.

- **Test data.** Each suite seeds the portal via its real REST endpoints
  (or via `jamsesh` subcommands where applicable) — **no direct DB
  inserts**. **Rationale**: keeps tests on the actual API path; preserves
  the org_id invariant; surfaces bugs in the seed path itself.

- **Determinism.** All time-sensitive code goes through a clock interface
  in the portal (already in use); chaos tests drive that interface via a
  test-only `/test/clock` endpoint behind a build tag — **not** wired in
  production builds. Where libfaketime is needed for subprocesses
  (cosign-signed binary verification, etc.), it's applied via
  `LD_PRELOAD` at the container level.

- **CI integration.** A new `make test-e2e` target builds the portal
  binary, runs the Go e2e suite (Testcontainers brings up dependencies
  per-test), then runs the Playwright suite. The release-deploy quality
  gate auto-invokes `e2e-test-design --audit --release <version>` to
  produce per-release coverage findings as items.

## Decomposition

Five child features. `infrastructure` is the foundation that the four
content layers consume. Golden + failure parallelize after infra lands;
chaos depends on golden (chaos verifies graceful degradation of paths
golden has already proven); fuzzing is independent of golden/chaos but
needs infra for the harnesses' I/O.

Critical path: `infrastructure → {golden || failure || fuzzing} → chaos`.
Four deep with three-way parallel in the middle band.

### Child features

- `epic-e2e-tests-infrastructure` — docker-compose stack +
  Testcontainers fixtures + `ccdriver` Go package + Playwright bootstrap
  + `make test-e2e` target + CI workflow + portal binary build artifact
  handling — depends on: `[]`
- `epic-e2e-tests-golden-path` — 3-5 golden-path Go spec files (join +
  push-and-merge, fork + comment, finalize-plan) + 3 Playwright golden
  journeys (login, session list, finalize) — depends on:
  `[epic-e2e-tests-infrastructure]`
- `epic-e2e-tests-failure-mode` — failure-mode Go specs across the
  catalog (invalid input, missing config, unavailable dep, boundary
  values, permission failures, interrupted operations) — depends on:
  `[epic-e2e-tests-infrastructure]`
- `epic-e2e-tests-chaos` — Toxiproxy + Pumba + libfaketime chaos
  scenarios (network jitter on portal↔DB, container pause on
  auto-merger, clock skew on token expiry, WS reconnection under drop)
  — depends on: `[epic-e2e-tests-golden-path]`
- `epic-e2e-tests-fuzzing` — property-based + grammar-based harnesses
  for commit-trailer parser, pre-receive validators (ref namespace + path
  scope), MCP tool input schemas, OAuth state token format — depends on:
  `[epic-e2e-tests-infrastructure]`

### Decomposition risks

- **Infrastructure is the bottleneck.** Everything else is gated on it
  landing cleanly. Design pass on `infrastructure` should produce an
  explicit acceptance bar: a single trivial test (e.g., "portal returns
  200 on `/healthz` via Testcontainers-Go stack") proves the entire
  scaffolding works end-to-end before any content layer starts.
- **Playwright flake risk.** Headless browser tests are the most
  flake-prone class in any e2e suite. The `golden-path` feature design
  should produce explicit retry/wait conventions, scoped to user-visible
  state transitions (not arbitrary `sleep` calls).
- **Chaos depends on production retry behavior actually existing.** The
  retry queue in plugin hooks, the WebSocket reconnect logic in the SPA,
  and the auto-merger conflict fallback are the chaos surface — if any
  of those are not yet implemented at design time, the chaos feature
  design must flag the gap and spawn an item back to that subsystem
  rather than write a tautology against a stubbed retry.
- **Fuzzing scope creep.** Property-based testing across every validator
  could swallow weeks. The fuzzing feature design should pick 3-4 parser
  surfaces with the highest bug-density / blast-radius product and stop
  there.
- **Plugin-driver fidelity drift.** The `ccdriver` Go harness is our
  Claude Code substitute. If Anthropic ships a behavior change in the
  hook protocol, the driver drifts silently. Mitigation: a single
  "contract" test that asserts the JSON shape against the spec in
  `docs/research/cc-plugin-hooks.md` (if it exists) or against a frozen
  example payload checked into `tests/e2e/fixtures/ccdriver/contract/`.

## Autopilot decision log

- 2026-05-17 — Advanced `drafting → implementing` directly without invoking
  `epic-design`. The decomposition was already produced by
  `e2e-test-design --bootstrap` (5 child features with depends_on chains,
  full design decisions in this body). Re-running `epic-design` would either
  no-op or risk duplicating children. The children are the work targets from
  here on.

## Acceptance criteria for the epic

- [ ] `tests/e2e/` exists with go.mod, fixture packages, and at least one
      green spec per taxonomy layer chosen
- [ ] `make test-e2e` runs the full Go suite + Playwright suite locally
- [ ] CI workflow runs `make test-e2e` on every PR (replacing the lone
      healthz smoke check)
- [ ] `release-deploy` invokes `e2e-test-design --audit --release
      <version>` and routes findings through `gate_origin: tests`
- [ ] The three in-process mocks (frontend `MockWebSocket`,
      `senders.Sender` httptest stub, `storage.Service` stub) are
      explicitly preserved in unit-test scope and **not** consumed by
      e2e

## Epic-level review (2026-05-17)

**Verdict**: Approve — epic delivered as briefed.

All 5 child features done. The e2e program is live:
- `epic-e2e-tests-infrastructure` (done) — `tests/e2e/` module, 5 Testcontainers fixtures, ccdriver, Playwright bootstrap, CI workflow
- `epic-e2e-tests-failure-mode` (done) — REST validation, config/dep failures, interrupted ops, SPA error states
- `epic-e2e-tests-golden-path` (done) — onboarding, session lifecycle, collab+auto-merger+MCP, fork+addressed comments, finalize+plan execution
- `epic-e2e-tests-fuzzing` (done) — pre-receive validators (3 harnesses), MCP tool input (property-based)
- `epic-e2e-tests-chaos` (done) — DB-latency tolerance, auto-merger pause resilience; 3 scenarios deferred to backlog dependencies

**Production bugs caught and fixed inline by the e2e program** (7 total):
1. `logging.statusRecorder.Unwrap()` missing → WS upgrade broken
2. `sessions.status` CHECK constraint missing `'finalizing'` → finalize-lock broken
3. `receive-pack` never seeded `draft` from `base` → auto-merger silently skipped merges for new sessions
4. `checkRefNamespace` allowed `..` traversal in branch segments → security gap
5. `gobwas/glob` panics on malformed pattern → DOS vector
6. `probeGlob` UTF-8 boundary panic → fixed during same fuzz run
7. Production Dockerfile lacked git binary → CreateSession failed in production

**Production bugs filed to backlog** (4):
- `portal-oauth-client-timeout` (no HTTP timeout, hangs on slow OAuth)
- `portal-dep-failure-error-codes` (`dep.*` codes not implemented)
- `portal-prod-dockerfile-base-image-review` (ops review needed)
- `bug-gobwas-glob-panic-on-malformed-pattern` (replace gobwas/glob)

**Capability check**: The brief's promised capability — "service-level e2e test program that exercises the portal and jamsesh binary as black-box artifacts" — is delivered. CI workflow runs `make test-e2e` on every PR. Playwright, Go specs, and fuzz harnesses all wired. Three in-process unit-test mocks preserved (MockWebSocket, senders stub, storage stub) — not consumed by e2e per the policy.

**Foundation-doc rolling-forward**: `docs/SELF_HOST.md` gained the OAuth env-var rows and section 12 (CI). Several follow-on docs items filed (scope-glob validation, dep-failure codes) — acceptable rolling-forward gaps.
