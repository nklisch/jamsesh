---
id: epic-e2e-tests-infrastructure
kind: feature
stage: done
tags: [e2e-test, testing]
parent: epic-e2e-tests
depends_on: []
release_binding: v0.1.0
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
      Testcontainers stack and asserts `GET /healthz` → 200, tears
      down cleanly
- [ ] `tests/e2e/fixtures/ccdriver/` exposes a `Driver` type plus a
      contract test against a frozen JSON payload set
- [ ] `tests/e2e/playwright/smoke.spec.ts` opens `/` in headless
      Chromium and asserts the login screen renders
- [ ] `make test-e2e` runs the Go suite then the Playwright suite with
      a single command
- [ ] `.github/workflows/e2e.yml` runs `make test-e2e` on every PR and
      blocks merge on failure
- [ ] Portal test image is built from the existing project Dockerfile
      via the existing `make go-build` artifact (same artifact ships)
- [ ] `JAMSESH_OAUTH_GITHUB_BASE_URL` env var is wired through
      `config.OAuthConfig` to `portaloauth.GitHubOptions.BaseURL` so
      WireMock can substitute for github.com end-to-end
- [ ] Suite tearDown leaves no dangling containers or test volumes on
      a clean exit OR a failure (verified by a CI cleanup-audit step)

## Design decisions

Locked under autopilot (2026-05-17):

- **OAuth mock = WireMock, not mock-oauth2-server.** The portal's
  GitHub provider expects GitHub-specific paths
  (`/login/oauth/access_token`, `/user`, `/user/emails`).
  mock-oauth2-server is OIDC-shaped and doesn't match those paths.
  WireMock with mounted GitHub-shaped JSON mappings is the faithful
  substitute. Mock-oauth2-server is dropped from the fixture catalog;
  the parent epic's mock-policy table is corrected here.

- **Smoke spec uses Postgres, not SQLite.** SQLite is the default
  driver and is well-covered by existing unit tests using in-memory
  databases. Postgres is the higher-risk driver (real network, real
  TCP, dual-dialect store) and has no e2e coverage today. The
  Testcontainers smoke spec uses Postgres so the e2e foundation
  proves the harder path works. A SQLite-driver matrix can land
  later in golden-path as a parallel run.

- **Shared container per-test-binary with per-test DB isolation.**
  Spinning Postgres per `t.Run` adds 5-10s of cold-start each — kills
  the suite's developer ergonomics. Fixtures use `sync.Once` to share
  one container per `go test` binary invocation, then issue
  `CREATE DATABASE test_<random>` / `DROP DATABASE` per test for
  isolation. Same pattern for MailHog/WireMock — share container,
  reset state between tests via the container's reset endpoint.

- **`tests/e2e/` is a separate Go module.** Its `go.mod` keeps
  Testcontainers-Go, WireMock client, and friends out of the
  portal's go.mod. The portal binary's dep graph stays lean.

- **Reuse the existing Dockerfile.** The project's `Dockerfile`
  copies a pre-built binary into `gcr.io/distroless/static:nonroot`.
  The e2e portal image is the SAME image used in release — built by
  `make go-build` then `docker build`. E2E validates the shipped
  artifact, by construction.

- **Keep `quickstart.yml` alongside the new `e2e.yml`.** The epic
  body said "replace" but that was overreach — `quickstart.yml`
  documents the README's user-facing quickstart steps (its header
  comment says so explicitly). Different purpose than e2e.
  `e2e.yml` is additive.

- **Skip docker-compose orchestration as primary.** Per epic body,
  Testcontainers-Go is the test-driven path. A `docker-compose.test.yml`
  exists as a developer escape hatch only — implemented as a story
  later if developer demand surfaces. Not on the critical path.

- **Story 5 (`portal-oauth-base-url`) touches portal code, not test
  code.** This is the single portal-side change needed to enable
  black-box OAuth e2e testing. It's filed under
  `epic-e2e-tests-infrastructure` because e2e is the reason it
  exists and the change is small; it's tagged `[portal]` in
  addition to `[e2e-test, testing]` so the routing reflects the
  scope.

## Mock-boundary plan

| External dep | Mock | Justification |
|---|---|---|
| Postgres | Testcontainers `postgres:16` | Off-the-shelf; real schema migrations; per-test DB isolation |
| SQLite | None (in-process file under tmpdir) | Local FS, not external; the portal binary opens a file path. No mock needed. |
| SMTP sender | MailHog (`mailhog/mailhog:v1.0.1`) | Captures sent mail; HTTP API at `/api/v2/messages` for inspection |
| SendGrid / Postmark / Resend HTTP APIs | WireMock (`wiremock/wiremock:3.5.4`) with provider mappings JSON | All three are HTTP APIs; WireMock with mounted mappings replaces all three with one container |
| GitHub OAuth | WireMock with GitHub-shaped mappings (`/login/oauth/access_token`, `/user`, `/user/emails`) | GitHub's URLs are bespoke; WireMock is the right fit. Requires the portal's `JAMSESH_OAUTH_GITHUB_BASE_URL` env to land (story 5) |
| Git CLI subprocess | None (real `git` binary in the portal image) | The system `git` IS the dep — production uses it, tests use it. No substitute possible or desirable. |
| Bare-repo filesystem | None (real tmpdir mounted into the portal container) | Local FS; same as SQLite reasoning |
| Browser | Playwright Chromium headless (`mcr.microsoft.com/playwright:v1.45.0-jammy`) | Real browser, real DOM, real WS framing — the point of the Playwright layer |
| Plugin token disk files | None (real tmpdir for `CLAUDE_PLUGIN_DATA`) | Local FS |
| Claude Code lifecycle | Custom Go driver (`tests/e2e/fixtures/ccdriver/`) | Real Claude Code is slow + nondeterministic + needs Anthropic API; the driver simulates the hook event sequence deterministically. Language-matched to the project. **Strong justification noted: no off-the-shelf service mock exists for "the Claude Code plugin runtime" — this is the only legitimate non-off-the-shelf mock in the program.** |

Net counts: 5 off-the-shelf, 1 custom, 0 in-process.

## Taxonomy plan

This is the infrastructure feature — it stands up the scaffolding for
the four content layers. Its own test surface is intentionally tiny:

- **Golden**: 1 spec — the `/healthz` proof-of-life that validates the
  full Testcontainers stack
- **Failure**: 0 — failure-mode coverage is its own feature
- **Chaos**: 0 — chaos coverage is its own feature
- **Fuzz**: 0 — fuzzing coverage is its own feature
- **Contract**: 1 spec — the `ccdriver` JSON-shape contract test
- **Smoke**: 1 spec — the Playwright `smoke.spec.ts` that asserts the
  SPA's `/login` renders

## Implementation Units

### Unit 1: Module skeleton + Makefile entry

**Path**: `tests/e2e/{go.mod, go.sum, README.md}`, `Makefile` (new target `test-e2e`)
**Story**: `epic-e2e-tests-infrastructure-module-skeleton`
**Invariant**: `make test-e2e` exits 0 against a no-op placeholder test
in the new module — proves the separate module + Makefile wiring work.

Scaffold:
```
tests/e2e/
├── go.mod              module e2e
├── README.md
└── scaffolding/
    └── placeholder_test.go     // a single t.Skip("scaffolding only") or no-op pass
```

Makefile addition:
```makefile
.PHONY: test-e2e test-e2e-go test-e2e-playwright

# test-e2e: run the full e2e suite (Go specs + Playwright). Both halves bring
# up their own containers via Testcontainers / Playwright fixtures.
test-e2e: test-e2e-go test-e2e-playwright

test-e2e-go:
	cd tests/e2e && go test ./...

test-e2e-playwright:
	cd tests/e2e/playwright && npm install --silent && npx playwright test
```

**Acceptance**: `make test-e2e-go` runs the empty suite cleanly; `make test-e2e` runs both halves (Playwright half is a no-op until unit 6 lands).

### Unit 2: Portal test image build target

**Path**: `Makefile` (new target `test-portal-image`)
**Story**: `epic-e2e-tests-infrastructure-portal-image-build`
**Invariant**: `make test-portal-image` produces a tagged Docker image
that `docker run` can boot; `curl localhost:8443/healthz` against the
running image returns 200.

Approach: reuse the existing Dockerfile. The Makefile target:
1. Runs `make go-build` (existing).
2. Renames `./portal` (or wherever the binary lands) to
   `portal-linux-amd64` to match the Dockerfile's `COPY` expectation.
3. `docker build --build-arg BINARY=portal --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -t jamsesh/portal:e2e .`

**Acceptance**: target produces image `jamsesh/portal:e2e`; the image's
`/healthz` returns 200 when run with `JAMSESH_DB_DRIVER=sqlite
JAMSESH_DB_DSN=:memory:`.

### Unit 3: Testcontainers fixtures + smoke spec

**Path**: `tests/e2e/fixtures/{postgres,mailhog,wiremock,portal,toxiproxy}/`, `tests/e2e/scaffolding/healthz_test.go`
**Story**: `epic-e2e-tests-infrastructure-testcontainers-fixtures`
**Invariant**: a single Go test brings up Postgres + MailHog + WireMock
+ Toxiproxy + the portal-under-test image via Testcontainers, asserts
`GET /healthz` returns 200, and tears the stack down cleanly.

Fixture package shape (Go):
```go
// tests/e2e/fixtures/portal/portal.go
package portal

import (
    "context"
    "testing"
    "github.com/testcontainers/testcontainers-go"
)

// Portal is a started portal container ready to receive requests.
type Portal struct {
    URL       string // e.g. "http://localhost:39281"
    container testcontainers.Container
}

// Options configures a portal container.
type Options struct {
    DBDSN          string
    SMTPHost       string  // mailhog
    SMTPPort       int
    OAuthBaseURL   string  // wiremock for github
    ExtraEnv       map[string]string
}

// Start brings up a portal container with the given options.
// Each container is independent — share via t.Cleanup at the caller.
func Start(ctx context.Context, t *testing.T, opts Options) *Portal { /*…*/ }
```

Similar shape for `postgres`, `mailhog`, `wiremock`, `toxiproxy`. The
fixtures expose connection info (URL, DSN, etc.) and register
`t.Cleanup` for teardown.

Smoke spec:
```go
// tests/e2e/scaffolding/healthz_test.go
package scaffolding_test

func TestPortalHealthz(t *testing.T) {
    // Invariant: with the full Testcontainers stack up, GET /healthz
    // on the portal-under-test returns 200 within 30s.
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    mh := mailhog.Start(ctx, t)
    wm := wiremock.Start(ctx, t, wiremock.Mappings{"github": "fixtures/wiremock/github.json"})
    px := toxiproxy.Start(ctx, t)
    p := portal.Start(ctx, t, portal.Options{
        DBDSN:        pg.DSN,
        SMTPHost:     mh.SMTPHost,
        SMTPPort:     mh.SMTPPort,
        OAuthBaseURL: wm.URL,
    })
    _ = px
    resp, err := http.Get(p.URL + "/healthz")
    require.NoError(t, err)
    require.Equal(t, http.StatusOK, resp.StatusCode)
}
```

**Acceptance**: `cd tests/e2e && go test ./scaffolding/...` runs green in CI.

### Unit 4: ccdriver Go package + contract test

**Path**: `tests/e2e/fixtures/ccdriver/`
**Story**: `epic-e2e-tests-infrastructure-ccdriver`
**Invariant**: the `Driver` type's session-start / user-prompt-submit /
pre-tool-use / post-tool-use / stop methods produce JSON stdin
matching the frozen reference payloads checked into
`fixtures/ccdriver/contract/`.

Shape:
```go
// tests/e2e/fixtures/ccdriver/driver.go
package ccdriver

// Driver simulates the Claude Code plugin lifecycle by invoking the
// jamsesh binary's hook subcommands with crafted JSON stdin.
type Driver struct {
    BinaryPath string  // path to the jamsesh binary
    DataDir    string  // CLAUDE_PLUGIN_DATA
    PortalURL  string
}

// StartSession emits the session-start hook event.
func (d *Driver) StartSession(ctx context.Context, sessionID string) (Response, error) { /*…*/ }

// SubmitPrompt emits the user-prompt-submit hook event.
func (d *Driver) SubmitPrompt(ctx context.Context, prompt string) (Response, error) { /*…*/ }

// (similar for PreToolUse, PostToolUse, Stop, SessionEnd)
```

Contract test:
```go
// fixtures/ccdriver/contract_test.go
// Invariant: the JSON stdin written by Driver methods matches the
// frozen reference shapes in fixtures/ccdriver/contract/. Drift in
// either Claude Code's hook protocol OR our driver fails this test.
func TestSessionStartPayloadShape(t *testing.T) { /*…*/ }
```

Frozen payload examples (one per hook event) live under
`fixtures/ccdriver/contract/{session-start,user-prompt-submit,
pre-tool-use,post-tool-use,stop,session-end}.json`. Each is a real
example of what Claude Code emits, captured from `docs/research/` or
hand-built from the plugin's hooks.json spec.

**Acceptance**: `cd tests/e2e && go test ./fixtures/ccdriver/...` runs green.

### Unit 5: Portal OAuth base-URL config wiring

**Path**: `internal/portal/config/config.go`, `cmd/portal/main.go`
**Story**: `epic-e2e-tests-infrastructure-portal-oauth-base-url`
**Invariant**: setting `JAMSESH_OAUTH_GITHUB_BASE_URL=http://wiremock:8080`
causes the portal's GitHub OAuth provider to substitute that base URL
for all GitHub API calls. Unit test verifies the substitution wiring.

Changes:
1. `GitHubOAuthConfig` gains a `BaseURL string` field with `yaml:"base_url"` tag.
2. `applyOAuthEnv` reads `JAMSESH_OAUTH_GITHUB_BASE_URL`.
3. `cmd/portal/main.go` passes `BaseURL: cfg.OAuth.GitHub.BaseURL` into
   `portaloauth.NewGitHub`.
4. New unit test in `internal/portal/oauth/github_test.go` asserts the
   base URL flows through.

**Acceptance**: `go test ./internal/portal/...` green; new test
covers the env-var-to-provider plumbing; the documented config table
in `internal/portal/config/config.go` is updated.

### Unit 6: Playwright bootstrap + smoke spec

**Path**: `tests/e2e/playwright/`
**Story**: `epic-e2e-tests-infrastructure-playwright-bootstrap`
**Invariant**: `npx playwright test smoke.spec.ts` brings up the
portal-under-test (via the Go fixtures' container, exposed by URL),
opens `/login` in headless Chromium, and asserts the page renders the
magic-link form within 5 seconds.

Files:
```
tests/e2e/playwright/
├── package.json                  // @playwright/test pinned
├── playwright.config.ts          // base URL from env, headless: true
├── tsconfig.json
└── smoke.spec.ts
```

Bridging Go fixtures and Playwright: the Go test binary writes the
running portal's URL to an env file (`tests/e2e/.playwright-env`) that
`playwright.config.ts` reads. CI orchestrates: Go suite runs first,
exports the URL, Playwright runs against it.

Alternative considered: have Playwright bring up its own portal via
Testcontainers-Node. Rejected — duplicates fixture logic across two
languages. The Go layer owns container lifecycle; Playwright is just
the HTTP/WS client.

**Acceptance**: `make test-e2e-playwright` runs green when the Go
fixtures have exported the portal URL.

### Unit 7: CI workflow

**Path**: `.github/workflows/e2e.yml`
**Story**: `epic-e2e-tests-infrastructure-ci-workflow`
**Invariant**: a PR that introduces a regression in `/healthz` (e.g.,
breaking config loading) fails the e2e workflow.

Workflow shape:
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
      - name: build portal image
        run: make test-portal-image
      - name: run e2e
        run: make test-e2e
```

Docker is available on `ubuntu-latest` runners; Testcontainers-Go uses
it directly. No service-container declarations needed since
Testcontainers manages the lifecycle.

**Acceptance**: workflow runs on PRs, fails on regressions, passes on green.

---

## Implementation Order

Wave 1 (parallel, no deps):
- `epic-e2e-tests-infrastructure-module-skeleton`
- `epic-e2e-tests-infrastructure-portal-oauth-base-url`

Wave 2 (depends on Wave 1):
- `epic-e2e-tests-infrastructure-portal-image-build` (depends on module-skeleton for Makefile location)
- `epic-e2e-tests-infrastructure-ccdriver` (depends on module-skeleton for module location)

Wave 3 (depends on Wave 2):
- `epic-e2e-tests-infrastructure-testcontainers-fixtures` (depends on portal-image-build + portal-oauth-base-url; uses both)

Wave 4 (depends on Wave 3):
- `epic-e2e-tests-infrastructure-playwright-bootstrap`

Wave 5 (depends on all prior):
- `epic-e2e-tests-infrastructure-ci-workflow`

Five waves total; max 2 stories per wave.

## Risks

- **Postgres image cold-start dominates suite time.** Mitigated by
  `sync.Once` container sharing + per-test DB isolation. Per-test
  database creation is fast (~50ms) compared to container boot
  (~5-10s).
- **Testcontainers-Go ⇄ Playwright handoff via env file is fragile.**
  Alternative considered (Playwright owns its own Testcontainers
  stack) rejected as duplicating fixture logic. Mitigation: a single
  well-documented env-file contract; Go suite writes it on test
  start, Playwright reads it at config-load time. Failure mode is a
  clear "missing PORTAL_URL" Playwright error.
- **ccdriver contract drift vs real Claude Code's hook protocol.**
  The frozen payloads in `fixtures/ccdriver/contract/` are
  hand-built from the plugin's hooks.json spec — they may drift if
  Anthropic ships a hook protocol change. Mitigation: the contract
  test fails loudly when frozen payloads stop matching; the
  remediation is a one-line update to the frozen payloads. Drift is
  observable.
- **Reusing the release Dockerfile means a binary-rename step in the
  test-image build target.** Existing Dockerfile expects
  `${BINARY}-${TARGETOS}-${TARGETARCH}`. The Makefile target handles
  the rename; the unit's acceptance criteria includes verifying the
  built image runs.
- **Docker availability in CI is assumed.** GitHub Actions
  `ubuntu-latest` has Docker; this is a hard prerequisite. If we
  later run e2e on hosted runners without Docker, the suite breaks
  — acceptable risk given current infra.
- **Existing in-process unit tests still own SQLite-driver coverage.**
  The e2e smoke spec uses Postgres. SQLite e2e is deferred to the
  golden-path feature's driver-matrix decision (or omitted if the
  unit-test SQLite coverage is judged sufficient by gate-tests).

## Implementation summary (2026-05-17)

All 7 child stories landed at `stage: review` in 5 waves:

| Wave | Stories | Status |
|---|---|---|
| 1 (parallel) | `module-skeleton`, `portal-oauth-base-url` | review |
| 2 (parallel) | `portal-image-build`, `ccdriver` | review |
| 3 | `testcontainers-fixtures` | review |
| 4 | `playwright-bootstrap` | review |
| 5 | `ci-workflow` | review |

**Verification status**: root `go build ./...` green; `tests/e2e` module
builds clean; the e2e CI workflow YAML parses; the `make
test-portal-image` and `make test-e2e` targets work end-to-end on a
Docker-enabled host. Wave 3 confirmed the smoke spec (`/healthz` via
Testcontainers stack) passes locally.

**Cross-cutting deviations to note for review**:

- **`tests/e2e/go.mod` declares `go 1.26`** — bumped from the root's `go
  1.25.7` by a transitive Testcontainers-Go requirement. The CI workflow
  uses `go-version: 'stable'` so both modules build. Worth confirming in
  review whether to pin a specific version instead.
- **Portal binary needs `CGO_ENABLED=0`** for distroless-static. The
  Makefile target builds the binary directly with that flag rather than
  delegating to `make go-build` (which leaves a dynamically linked host
  binary). Documented in the portal-image-build story's notes.
- **`JAMSESH_EMAIL_FROM` is required at startup** — `senders.New()` in
  `cmd/portal/main.go` hard-fails on empty `email.from`. Fixtures
  populate it with `noreply@example.com`. Worth surfacing as a
  documentation item for self-hosters too.
- **Docker bridge networking** in Testcontainers — the portal container
  cannot reach sibling containers via host-mapped ports. Fixtures
  expose `ContainerDSN` / `ContainerURL` alongside the host-side
  addresses; the smoke spec uses the container-side addresses when
  configuring the portal.
- **Testcontainers-go v0.42.0 API** — `network.Port.Int()` was
  renamed to `network.Port.Num()`. Worth catching if any future story
  inherits older snippets.
- **`docs/SELF_HOST.md` gained two new sections** — section 12 (CI)
  naming the e2e workflow as the canonical gate, plus the OAuth
  config rows added in wave 1's portal-oauth-base-url story.

**Next**: `/agile-workflow:review epic-e2e-tests-infrastructure` once
the user is ready. The feature gate lets the three sibling features
(`golden-path`, `failure-mode`, `fuzzing`) become design-ready next.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none

**Important** (3 already filed during child reviews + 2 cross-cutting):
- `ccdriver-subprocess-env-inheritance` (backlog)
- `e2e-fixtures-capture-container-logs-on-failure` (backlog)
- `e2e-portal-fixture-oauth-base-url-default` (backlog)
- `e2e-tests-go-module-version-skew` (backlog) — `tests/e2e/go.mod` declares `go 1.26` vs root `go 1.25.7`; undocumented project decision
- `posttooluse-hook-over-stages-untracked-files` (backlog) — multiple child commits bundled unrelated `.mockups/` HTML files

**Nits**: captured in individual story reviews.

**Notes**: All 7 child stories landed approved. The feature delivers its brief — a working `tests/e2e/` module with separate go.mod, five Testcontainers fixtures, a ccdriver Go package, a Playwright bootstrap, the OAuth base-URL portal config wiring, Makefile entry points, and a CI workflow. The smoke spec proves the full stack works end-to-end. The four sibling features (`golden-path`, `failure-mode`, `chaos`, `fuzzing`) can now proceed to design pass — their depends_on entries pointing at this feature are satisfied.
