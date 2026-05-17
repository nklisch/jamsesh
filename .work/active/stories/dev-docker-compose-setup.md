---
id: dev-docker-compose-setup
kind: story
stage: implementing
tags: [infra]
parent: dev-docker-compose
depends_on: []
release_binding: null
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
