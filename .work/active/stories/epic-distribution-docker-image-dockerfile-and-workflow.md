---
id: epic-distribution-docker-image-dockerfile-and-workflow
kind: story
stage: implementing
tags: [infra]
parent: epic-distribution-docker-image
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Docker Image — Dockerfile + Release Workflow Extension

## Scope

Add Dockerfile + docker job to release.yml.

## Units delivered

- `Dockerfile` — distroless static + COPY-from-artifact
- `.github/workflows/release.yml` (edit) — `docker` job after `build`

## Acceptance Criteria

- [ ] `actionlint` clean on updated release.yml
- [ ] Local `docker buildx build` with stub binaries succeeds
- [ ] Workflow tagged-push triggers docker build/push/sign chain
- [ ] Image runs `/usr/local/bin/portal --version` cleanly

## Notes

- Distroless image uses nonroot user; portal binary must be world-executable.
- Cosign image signing uses keyless OIDC; same trust anchor as binary signing.
