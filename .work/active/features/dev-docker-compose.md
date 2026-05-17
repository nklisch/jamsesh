---
id: dev-docker-compose
kind: feature
stage: drafting
tags: [infra]
parent: null
depends_on: []
release_binding: null
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

## Open questions for feature-design

- Which file-watch tool: `air`, `reflex`, `entr`, or `wgo`? The Go
  ecosystem has several with different reliability profiles. Pick one
  and pin it; document why.
- Should the dev compose name its containers (e.g. `jamsesh-portal-dev`)
  or rely on Compose's default project naming? Named containers are
  easier to `docker exec` into; project-default keeps it portable.
- Does the `.data/` directory live in the host CWD, in a Compose-managed
  named volume, or both via a config flag? Affects whether `git clean`
  nukes the dev DB.
- Does the dev compose preload any fixture data (an empty org, a sample
  session) so a fresh `up` lands somewhere usable? Probably no for v1
  (lean minimal), but worth a note.

## History

Sourced from `.work/backlog/idea-docker-compose-local-dev.md` (parked in
this session via `/agile-workflow:park`).
