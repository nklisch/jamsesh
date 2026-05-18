---
id: epic-distribution-docker-image-dockerfile-and-workflow
kind: story
stage: done
tags: [infra]
parent: epic-distribution-docker-image
depends_on: []
release_binding: v0.1.0
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

## Implementation notes

- `Dockerfile`: Added `ARG BINARY` re-declaration after `FROM` so it's visible
  in the build stage (Docker pre-FROM args don't survive into the stage without
  re-declaration). The pre-FROM `ARG BINARY=portal` sets the default; the
  post-FROM `ARG BINARY` inherits that default into the stage.
- `docker` job added to `.github/workflows/release.yml` after `sign-and-release`,
  triggered only on `refs/tags/v*` pushes. Downloads build artifacts, assembles
  `docker-context/` with both linux arch binaries + Dockerfile, runs
  `docker/build-push-action@v6` with `platforms: linux/amd64,linux/arm64`,
  then signs the image digest with cosign keyless OIDC.
- `actionlint` passes clean.
- Local `docker buildx build --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64
  --build-arg BINARY=portal` with stub binary succeeds and produces a valid image.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Dockerfile ARG re-declaration after FROM is the right pattern. cosign image signing keyed off the build digest. Multi-arch via buildx + QEMU is standard.
