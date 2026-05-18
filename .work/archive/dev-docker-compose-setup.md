---
id: dev-docker-compose-setup
kind: story
stage: done
tags: [infra]
parent: dev-docker-compose
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Setup — `Dockerfile.dev` + `compose.yaml` + air config

The working `docker compose up` path. Brings up the portal binary on
`localhost:8443` with `air`-driven hot rebuild against host-mounted source
and SQLite-backed data in `./.data/`.

## Files

- New: `/Dockerfile.dev`
- New: `/.air.toml`
- New: `/compose.yaml` (modern Compose Specification name)
- New: `/.dockerignore`
- Modify: `/.gitignore` (add `.data/` and `tmp/`)

## Target shape

### `/Dockerfile.dev`

```dockerfile
# Dockerfile.dev — local-development image with the Go toolchain and `air`
# for file-watched rebuilds. NOT used for production releases — see the
# top-level `Dockerfile` for the distroless release image.
FROM golang:1.24-alpine AS dev

RUN apk add --no-cache git ca-certificates && \
    go install github.com/air-verse/air@v1.61.0

WORKDIR /src

# Pre-warm the build cache by copying go.mod / go.sum and downloading deps.
# This means `compose up --build` after a `go.mod` edit re-downloads; pure
# code edits skip this layer.
COPY go.mod go.sum ./
RUN go mod download

EXPOSE 8443

# air watches /src (bind-mounted from the host) and rebuilds on .go changes.
CMD ["air", "-c", ".air.toml"]
```

Pin the Go version to match `go.mod`'s declared toolchain. If `go.mod`
declares `go 1.23` or similar, use the matching `golang:1.23-alpine` base.
Pin `air` to a specific tag (the snippet uses v1.61.0 — verify against
github.com/air-verse/air's current release at implementation time).

Why alpine, not distroless: dev needs the Go toolchain at runtime to
rebuild on file change. The distroless release image is a separate
concern handled by the existing top-level `Dockerfile`.

### `/.air.toml`

```toml
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/portal ./cmd/portal"
  bin = "./tmp/portal"
  include_ext = ["go"]
  exclude_dir = [
    "frontend",
    "node_modules",
    ".git",
    ".data",
    "tmp",
    "tests/e2e/playwright/node_modules",
    "tests/e2e/playwright/playwright-report",
    "tests/e2e/playwright/test-results",
    ".mockups",
  ]
  delay = 500             # ms debounce
  stop_on_root = true

[log]
  time = false
```

Notes:
- `cmd/portal` is the binary entry point (see existing project layout).
- We deliberately do NOT run `make build` or `make generate` — those
  regenerate frontend assets and sqlc output; the dev loop assumes those
  have already been generated (developer ran them once host-side) and
  the embed dir is either populated or empty (acceptable, the dev SPA is
  served by Vite host-side at `:5173`).
- The embed dir `internal/portal/assets/dist/.gitkeep` exists, so the
  portal compiles cleanly even when the SPA hasn't been built.

### `/compose.yaml`

```yaml
# Local-development orchestration. NOT for production — see the top-level
# Dockerfile and docs/SELF_HOST.md for the production deployment path.
# Reference: `dev-docker-compose` feature in .work/.

services:
  portal:
    build:
      context: .
      dockerfile: Dockerfile.dev
    ports:
      - "8443:8443"
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
      # Allow the Vite dev server to connect via WebSocket.
      JAMSESH_WS_ALLOW_ORIGINS: http://localhost:5173
```

Notes on env var values:
- `JAMSESH_BIND=:8443` matches the production default and Vite's already-
  configured proxy in `frontend/vite.config.ts`.
- `JAMSESH_TLS_MODE=behind_proxy` runs plain HTTP, no certs.
- `JAMSESH_LOG_LEVEL=-4` is debug level (per the comment in
  `internal/portal/config/config.go:35`).
- `JAMSESH_WS_ALLOW_ORIGINS=http://localhost:5173` is required — the WS
  gateway denies connections by default; the developer-served Vite frontend
  needs this origin allowed.
- Names of env vars verified against `internal/portal/config/config.go`
  (lines 245+). If any name drifts, update both sides.

**Permission note (UID/GID)**: by default, Alpine containers run as root.
Files written to the bind-mounted `./.data/` will be root-owned on the
host. To avoid `chown` annoyance, this initial story does NOT add a
`user:` directive — accept root ownership for v1. If it becomes a pain
point, a follow-up adds `user: "${UID}:${GID}"` with an env-var pattern.

### `/.dockerignore`

```
# Files NOT to copy into the Docker build context.
# Note: the dev compose bind-mounts the source tree at runtime, so this
# only affects the `Dockerfile.dev` build step (which COPYs only go.mod/go.sum).

.git/
.gitignore
.dockerignore
node_modules/
frontend/node_modules/
frontend/dist/
internal/portal/assets/dist/
.data/
tmp/
.mockups/
.work/
.claude/
docs/

# Compiled binaries (gitignored anyway, but in case)
/portal
/jamsesh
*.db
*.db-journal
*.db-shm
*.db-wal
```

### `/.gitignore` (additions)

Append to the existing `.gitignore`:

```
# Local dev compose data
.data/

# air build artifacts
tmp/
```

## Acceptance criteria

- [ ] `docker compose up` from a fresh clone builds the dev image and
      brings up the portal listening on `localhost:8443`
- [ ] `curl http://localhost:8443/healthz` returns `{"status":"ok"}` (or
      whatever the actual healthz body is — verify against the existing
      `/healthz` handler)
- [ ] Editing any `.go` file under the host source tree triggers an `air`
      rebuild + restart within ~3 seconds; the log shows `air` reloading
- [ ] `docker compose down` stops cleanly; `docker compose down -v` also
      drops the `./.data/` contents (or at least leaves them visible on
      the host so the developer can `rm -rf .data` deliberately)
- [ ] Vite-served frontend on `:5173` can connect via WebSocket to the
      portal — verify by running `cd frontend && npm run dev` and opening
      the app in a browser; the dev console should NOT show
      "WebSocket connection denied" errors
- [ ] `.data/` and `tmp/` are gitignored

## Risk

LOW-MEDIUM. The riskiest part is `air`'s reliability on bind-mounted
source on macOS/Windows (FS notifications can be flaky). Mitigations:
- `air` has a polling mode (`poll = true` in `[build]`) — implementer can
  add it if file-watch misses changes
- Document the override in case of port conflicts
  (`JAMSESH_BIND=:9443 docker compose up`)

The Go toolchain version pin matters — using a wrong version surfaces
as a build error on first `compose up --build`.

## Rollback

`git revert` the commit. The 5 new files are removed; the .gitignore
addition is reverted. Existing `make build` workflows are unaffected.

## Implementation notes

### What landed

- `Dockerfile.dev` — `golang:1.25-alpine` + `air@v1.61.0` + dep pre-warm layer
- `.air.toml` — watches `.go` files, debounce 500ms, excludes frontend/test dirs
- `compose.yaml` — portal service with bind-mounted source, `.data/` for SQLite
- `.dockerignore` — trims the build context for the `COPY go.mod go.sum` step
- `.gitignore` — appended `.data/` and `tmp/`
- `cmd/portal/main.go` — wired `JAMSESH_WS_ALLOW_ORIGINS` env var (see deviation below)

### Deviations from spec

**Go toolchain version**: Spec showed `golang:1.24-alpine`; `go.mod` declares `go 1.25.7`.
Used `golang:1.25-alpine` as the story instructs: "Pin Go version to match `go.mod`'s declared toolchain."

**Wired JAMSESH_WS_ALLOW_ORIGINS env var to wsgateway.Gateway.AllowOrigins in
`cmd/portal/main.go`**: The comment described it as configurable but the value was
hardcoded `[]string{}`; required for the Vite WS acceptance criterion. Added `"strings"`
import and a parsing loop (comma-separated, trimmed, empty entries dropped).

**Added `-buildvcs=false` to `.air.toml` build cmd**: `go build` inside the container
reads VCS metadata but the Alpine git in the container can't access the host-mounted
`.git` safely (`exit status 128`). The flag suppresses VCS stamping for the dev build;
production image (`Dockerfile`) uses a pre-built binary and is unaffected.

**Added `JAMSESH_EMAIL_FROM: dev@localhost` to `compose.yaml`**: `senders.New()` returns
a hard error when `email.from` is empty, causing `os.Exit(1)` before the portal
can listen. The dev compose sets a stub address so the server starts; actual SMTP
delivery fails gracefully (no local MTA). Developers who need email in dev can
overlay their own provider env vars.

### Verification results

1. `go build ./...` — clean (0 errors)
2. `go test ./cmd/portal/... ./internal/portal/wsgateway/...` — `wsgateway` PASS, `cmd/portal` has no test files
3. `docker compose up --build -d` — built image successfully, container started
4. `curl -fsS http://localhost:8443/healthz` — returned `{"status":"ok"}`
5. `touch internal/portal/router/router.go` — air detected change, logged "building...",
   rebuilt and restarted within ~5s; `portal listening` message appeared in logs
6. `docker compose down -v` — stack stopped cleanly; `.data/jamsesh.db` visible on host

All acceptance criteria verified automated except the Vite WS criterion (requires
running browser + frontend dev server; manual-verify only).

## Review (2026-05-17)

**Verdict**: Approve with comments

**Blockers**: none

**Important**:
- `docs/SELF_HOST.md` configuration table missing `JAMSESH_WS_ALLOW_ORIGINS`
  row. The env var is now load-bearing in production code; the doc table is the
  canonical operator-facing reference and the inline comment in
  `cmd/portal/main.go:319-321` points there. Pre-existing gap that this story
  makes more visible rather than introducing — filed as backlog item
  `docs-self-host-document-ws-allow-origins`, not a blocker.
  → Item: `.work/backlog/docs-self-host-document-ws-allow-origins.md`
- The new env-var parsing in `cmd/portal/main.go:320-328` has no unit test.
  The parsing is small but production-touching (security-relevant: WS origin
  allowlist); a table test of the comma-split/trim/empty-drop behavior would
  earn confidence cheaply. Filed as backlog item.
  → Item: `.work/backlog/portal-test-ws-allow-origins-env-parsing.md`

**Nits**:
- The `JAMSESH_EMAIL_FROM: dev@localhost` workaround in `compose.yaml` is
  pragmatic and well-commented. Suggests `senders.New()` validation might
  be too strict for dev mode (out of scope here; future consideration).
- `-buildvcs=false` in `.air.toml` is a clean workaround for the container
  /host-mounted-`.git` permission mismatch and is documented at the call
  site in the commit message.

**Notes**: All acceptance criteria pass except the Vite WS browser-side check
(manual-verify only). The deviations (Go 1.25 pin, WS env-var wiring,
EMAIL_FROM stub, `-buildvcs=false`) are all defensible and individually
documented in the implementation notes above.
