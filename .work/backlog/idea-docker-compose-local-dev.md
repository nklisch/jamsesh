---
id: idea-docker-compose-local-dev
created: 2026-05-17
tags: [infra]
---

A `docker-compose.yml` (or `compose.yaml`) at the repo root for spinning up
a local dev environment in one command. Distinct from the existing
`Dockerfile` + `epic-distribution-docker-image` work, which targets the
release artifact. The dev compose would orchestrate whatever services a
contributor needs running locally to exercise the portal end-to-end —
likely the portal binary itself (with file-watch / hot rebuild),
optionally a Postgres service as an alternative to the default SQLite
store, and possibly a frontend dev-server container. Goal: lower the
onboarding friction below "clone + read CLAUDE.md + run several
commands in the right order" to "clone + `docker compose up`".
