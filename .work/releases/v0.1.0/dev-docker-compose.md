---
id: dev-docker-compose
kind: feature
stage: done
tags: [infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Feature — Local-dev `docker compose`

## Brief

A `docker-compose.yml` (or `compose.yaml`) at the repo root that spins up
the jamsesh portal locally in one command. Goal: lower onboarding friction
below "clone + read CLAUDE.md + run several commands in the right order"
to "clone + `docker compose up`" — at least for the portal half of the
stack.

Distinct from the existing `epic-distribution-docker-image` work, which
produces the release-artifact Dockerfile that ships in marketplace. This
feature is the dev-time equivalent: file-watch hot rebuild, host-mounted
source for fast iteration, default SQLite store, dev-friendly logging.
The two Dockerfiles share nothing structurally — production is a minimal
distroless image; dev is a fat builder image with the Go toolchain.

## Strategic decisions

Locked in via `/agile-workflow:scope` Phase 1.7:

- **DX scope**: portal-only with SQLite, host-side frontend. The compose
  brings up the portal binary against a host-mounted source tree with
  Go file-watch rebuilds; SQLite persists to a named volume. Developers
  continue to run `cd frontend && npm run dev` host-side for hot frontend
  reload. Rationale: minimal moving parts, fastest iteration loop, no
  Postgres-vs-SQLite confusion at first onboarding. Postgres + a frontend
  container are explicit non-goals for v1; an "optional profiles" follow-up
  can layer them in later without rewriting v1.

## Out of scope (deliberately deferred)

- Postgres service in compose (use SQLite default; the dual-dialect sqlc
  setup keeps both paths viable)
- Frontend dev server in compose (`npm run dev` host-side is friction-free)
- Production-equivalent compose for multi-service local prod (not the
  goal — `epic-distribution-docker-image` owns that surface via the
  distroless image and the marketplace publish workflow)
- One-command full stack tear-up for E2E tests (the `epic-e2e-tests`
  work uses Testcontainers for its own stack-up; this feature targets
  developer-loop iteration, not test orchestration)

## What "done" looks like

- `docker compose up` from a fresh clone brings up the portal listening
  on `:8080` with SQLite-backed data
- Editing a `.go` file under the host-mounted source triggers a rebuild
  and restart inside the container within a couple of seconds (via `air`,
  `reflex`, or equivalent — implementer picks)
- `docker compose down` cleanly stops everything; `docker compose down -v`
  also drops the data volume for a fresh slate
- A short section in `README.md` (or `docs/SELF_HOST.md` — whichever is
  closer to the onboarding flow) documents the new path: "Quick start:
  `docker compose up`; for hot frontend reload, in another terminal
  `cd frontend && npm run dev`"
- The compose-managed SQLite database is at a well-known mount point so
  `sqlite3 ./.data/jamsesh.db` (or similar) works from the host for
  debugging
- No regression to existing `make build` / `make go-build` / host-side
  workflows

## Affected code areas (for feature-design grounding)

- Root: new `docker-compose.yml` (or `compose.yaml` — pick the modern
  Compose Specification name)
- Root: new `Dockerfile.dev` for the file-watch + host-mount setup
  (separate from the existing production `Dockerfile`)
- Root: possibly a `.dockerignore` augmentation if the existing one is
  too narrow for dev mode (the dev compose mounts the source tree, so
  ignore patterns matter less, but the build context still applies)
- `Makefile`: optional new `make dev` target that wraps `docker compose up`
- `README.md` or `docs/SELF_HOST.md`: a new onboarding subsection

## Design decisions

Locked in via `feature-design` Phase 4.5:

- **Data location**: host CWD `./.data/` (bind-mounted from repo root into
  the container at `/data`). Tradeoff accepted: `git clean -fdx` could nuke
  the dev DB; that's fine for a dev-only path. Add `.data/` to `.gitignore`.
- **Seed data**: no seed on `up`. Fresh compose starts empty; first action
  is signup or org-creation through the running portal. Matches production
  behavior; no schema-drift maintenance overhead.

Resolved with judgment during feature-design (implementation-time calls,
documented for traceability):

- **File-watch tool**: `air` (github.com/air-verse/air). Most popular Go
  file-watcher, well-maintained, single-binary install via `go install`,
  simple TOML config. Pinned to a specific tag in the Dockerfile.
- **Container naming**: Compose project-default (e.g. `jamsesh-portal-1`
  based on the repo directory name). No hardcoded names — keeps the
  compose file portable.

## Architectural choice

**Multi-stage build NOT needed for dev — single-stage alpine + Go
toolchain + `air` + bind-mounted source.**

Rationale: dev needs the Go toolchain at runtime (to rebuild on file
change). The production `Dockerfile` uses distroless-static for size and
security; the dev `Dockerfile.dev` accepts a larger image (~500MB) to
get fast iteration. The two Dockerfiles share no structure — they
optimize for different things.

Alternatives considered:
- **`go run` without file-watch**: simpler, but every change requires
  `docker compose restart portal`. Rejected — iteration loop too slow.
- **`air` running host-side, compose only for portal-binary execution**:
  multi-tool, half-host half-container; reject as awkward.
- **Multi-stage Dockerfile with a build stage + runtime stage**: useful
  for production; pure overhead for dev where the toolchain IS the
  runtime.

## Implementation Units

### Unit 1: `Dockerfile.dev`
**File**: `/Dockerfile.dev`
**Story**: `dev-docker-compose-setup`

```dockerfile
FROM golang:1.24-alpine AS dev
RUN apk add --no-cache git ca-certificates && \
    go install github.com/air-verse/air@v1.61.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
EXPOSE 8443
CMD ["air", "-c", ".air.toml"]
```

Pin Go version to match `go.mod`'s declared toolchain. The `go.mod` /
`go.sum` pre-copy + `go mod download` warms the module cache so pure
code edits don't re-download deps.

---

### Unit 2: `.air.toml`
**File**: `/.air.toml`
**Story**: `dev-docker-compose-setup`

```toml
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/portal ./cmd/portal"
  bin = "./tmp/portal"
  include_ext = ["go"]
  exclude_dir = ["frontend", "node_modules", ".git", ".data", "tmp",
                 "tests/e2e/playwright/node_modules",
                 "tests/e2e/playwright/playwright-report",
                 "tests/e2e/playwright/test-results", ".mockups"]
  delay = 500
  stop_on_root = true
```

The build command is `go build`, NOT `make build` — `make build` would
also rebuild the frontend, which is owned by the host-side Vite process
in dev mode. The portal's `//go:embed all:dist` embeds whatever's there
(possibly empty); the dev SPA is served by Vite on `:5173`.

---

### Unit 3: `compose.yaml`
**File**: `/compose.yaml`
**Story**: `dev-docker-compose-setup`

```yaml
services:
  portal:
    build: { context: ., dockerfile: Dockerfile.dev }
    ports: ["8443:8443"]
    volumes:
      - .:/src
      - ./.data:/data
    working_dir: /src
    environment:
      JAMSESH_BIND: ":8443"
      JAMSESH_TLS_MODE: behind_proxy
      JAMSESH_DB_DRIVER: sqlite
      JAMSESH_DB_DSN: /data/jamsesh.db
      JAMSESH_STORAGE: /data/storage
      JAMSESH_LOG_FORMAT: text
      JAMSESH_LOG_LEVEL: "-4"
      JAMSESH_WS_ALLOW_ORIGINS: http://localhost:5173
```

`JAMSESH_WS_ALLOW_ORIGINS=http://localhost:5173` is critical — without
it the Vite-served frontend's WebSocket subscriptions are rejected. The
existing `frontend/vite.config.ts` proxies `/api`, `/ws`, `/git`, `/mcp`
to `localhost:8443`, so the frontend → portal direction is already wired;
only the backend's WS allow-origin needs configuration.

---

### Unit 4: `.dockerignore`
**File**: `/.dockerignore` (new file — none exists today)
**Story**: `dev-docker-compose-setup`

Excludes the heavy directories (`node_modules`, `frontend/dist`,
`internal/portal/assets/dist`, `.data`, `tmp`, `.mockups`, `.work`,
`.claude`, `docs`) from the Docker build context. The bind-mount at
runtime overrides this for the live source; this only affects the
`Dockerfile.dev`'s initial `COPY go.mod go.sum` step.

---

### Unit 5: `.gitignore` additions
**File**: `/.gitignore` (modify existing)
**Story**: `dev-docker-compose-setup`

Append:

```
# Local dev compose data
.data/

# air build artifacts
tmp/
```

---

### Unit 6: `Makefile` `dev` targets
**File**: `/Makefile`
**Story**: `dev-docker-compose-docs`

Adds `dev`, `dev-down`, `dev-down-v`, `dev-rebuild` targets wrapping the
compose commands. `dev-down-v` ALSO removes `./.data/` so a clean reset
is one command.

---

### Unit 7: README onboarding section
**File**: `/README.md`
**Story**: `dev-docker-compose-docs`

New "Local development" section above the existing "Quickstart (Docker)"
operator section. Two-terminal walkthrough: `docker compose up` for the
portal, `cd frontend && npm run dev` for the SPA. Documents the `:5173`
browse URL, the `.data/` location, and the `make dev-down-v` reset.

The existing operator quickstart heading may need renaming (e.g.
"Operator quickstart") to disambiguate from the new dev section.

## Implementation Order

```
Wave 1 (1 agent): dev-docker-compose-setup     [no deps]
Wave 2 (1 agent): dev-docker-compose-docs      [← setup]
```

Two stories with linear dep. The docs story depends on setup because
the README references real artifacts (the compose service name, the
default port, the data location) — writing docs against a working setup
ensures accuracy.

## Testing

This feature has no automated unit tests — it's developer infrastructure.
Acceptance is verified by manual smoke tests captured in each story's
acceptance criteria. Specifically:

- **Setup story**: `docker compose up` brings up portal on `:8443`;
  `curl http://localhost:8443/healthz` returns OK; editing a `.go` file
  triggers rebuild; Vite frontend on `:5173` connects via WebSocket
  without rejection.
- **Docs story**: a fresh-clone walkthrough of the README "Local
  development" section lands at a working portal + SPA in under 2
  minutes (build time excluded — first-build cold cache may take longer).

## Risks

From the pre-mortem:

- **`air` file-watch reliability on macOS/Windows bind-mounts**. Native
  FS notifications can be flaky through Docker Desktop's VM layer.
  Mitigation: implementer can enable `poll = true` in `.air.toml`'s
  `[build]` section as a fallback if changes don't trigger rebuilds
  reliably.
- **Port :8443 conflict on the host**. Common port; might collide with
  another service. Mitigation: document the override pattern
  (`docker compose up` after exporting an alternative env, or compose's
  `--scale`/port-override flag).
- **Root-owned files in `./.data/`**. Alpine containers run as root by
  default; bind-mounted writes are root-owned on the host. Accepted as
  v1 friction; a follow-up can add `user: "${UID}:${GID}"` if the
  pattern becomes painful.
- **Go-version drift between `Dockerfile.dev` and `go.mod`**. Mitigation:
  implementer verifies and pins the version on first build.

## History

Sourced from `.work/backlog/idea-docker-compose-local-dev.md` (parked in
this session via `/agile-workflow:park`).

## Implementation summary

Both child stories shipped via `/agile-workflow:autopilot` in a 2-wave
orchestrator run:

- `dev-docker-compose-setup` → `stage: review` (commit `8d0e04e`). Landed
  `Dockerfile.dev` (golang:1.25-alpine pinned to `go.mod`'s 1.25.7), `.air.toml`
  (with `-buildvcs=false` for in-container git compat), `compose.yaml`,
  `.dockerignore`, and `.gitignore` additions for `.data/` and `tmp/`. Also
  wired `JAMSESH_WS_ALLOW_ORIGINS` in `cmd/portal/main.go` (the comment had
  described it as configurable but the slice was hardcoded — the Vite WS
  acceptance criterion required the wiring). Added `JAMSESH_EMAIL_FROM:
  dev@localhost` in compose env to satisfy `senders.New()` startup
  validation; actual email delivery fails gracefully in dev.
- `dev-docker-compose-docs` → `stage: review` (commit `8b28117`). Added
  `make dev`, `dev-down`, `dev-down-v`, `dev-rebuild` targets; renamed the
  README's `Quickstart (Docker)` heading to `Operator quickstart` and
  inserted a `Local development` section above it documenting the
  two-terminal flow, `./.data/` data path, and `make dev-down-v` reset.
  Deliberately skipped a reverse cross-link in `docs/SELF_HOST.md` —
  that doc is operator-facing.

Verification: `go build ./...` clean, `wsgateway` tests green,
`docker compose up --build` brought the portal up on `:8443`,
`curl /healthz` returned `{"status":"ok"}`, `.go` file edits triggered
air rebuilds within ~5s. The Vite WS criterion is manual-verify (requires
a browser).

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none (per-line concerns already filed during child reviews)

**Notes**: Both child stories already reviewed individually (setup: Approve
with comments, 2 backlog items filed; docs: Approve). Capability check
against the feature's "What done looks like": `docker compose up` brings
the portal up on :8443 (story corrected the brief's :8080 typo); `.go`
edits trigger rebuilds via air; `make dev-down-v` cleans the data volume;
the README documents the contributor path; the operator's `make build`
flow is unaffected. The realized decomposition matches the design (two
linear waves, setup → docs). Cross-cutting foundation-doc gap
(`JAMSESH_WS_ALLOW_ORIGINS` missing from `docs/SELF_HOST.md`) was already
captured during setup's review — not refiling.

Now possible: `git clone && docker compose up` is the new front door for
contributors. Onboarding friction drops from "read CLAUDE.md, run several
commands in the right order" to "two commands, two terminals".
