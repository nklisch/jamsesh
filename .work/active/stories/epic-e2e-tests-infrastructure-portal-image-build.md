---
id: epic-e2e-tests-infrastructure-portal-image-build
kind: story
stage: implementing
tags: [e2e-test, testing]
parent: epic-e2e-tests-infrastructure
depends_on: [epic-e2e-tests-infrastructure-module-skeleton]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Infra — Portal test image build target

## Scope

Add a Makefile target that builds the portal Docker image
(`jamsesh/portal:e2e`) from the existing project Dockerfile +
`make go-build` output. The image is what Testcontainers boots in
later stories.

## Background

The repo root `Dockerfile` is distroless-static and expects a
pre-built binary named `${BINARY}-${TARGETOS}-${TARGETARCH}`. The
existing release pipeline produces this; the e2e build flow needs
to produce it too, from a fresh local `make go-build`.

## Files to create / modify

- `Makefile` — add targets:
  - `test-portal-image` — builds the image
  - `test-portal-image-clean` — removes the built image (developer
    convenience)
- `tests/e2e/scaffolding/portal_image_test.go` — new test that runs
  `docker run --rm -e JAMSESH_DB_DRIVER=sqlite
  -e JAMSESH_DB_DSN=:memory: -e JAMSESH_TLS_MODE=behind_proxy
  -p 0:8443 jamsesh/portal:e2e`, polls `/healthz`, confirms 200, kills
  the container (skip the test if Docker is unavailable or the image
  isn't present, so the suite is still runnable without `make
  test-portal-image` first)

## Acceptance criteria

- [ ] `make test-portal-image` produces the tagged image (verifiable
      via `docker images jamsesh/portal:e2e`)
- [ ] The image runs as a non-root user (inherited from the base
      distroless image)
- [ ] The image responds to `/healthz` with 200 within 10 seconds of
      `docker run`
- [ ] `tests/e2e/scaffolding/portal_image_test.go` verifies the
      contract, skipping cleanly when the image is absent
- [ ] `make test-portal-image-clean` removes the tag

## Notes for the implementer

- The existing Dockerfile uses `ARG BINARY=portal` and `ARG TARGETOS`
  + `ARG TARGETARCH` with `COPY ${BINARY}-${TARGETOS}-${TARGETARCH}`.
  The local build produces a binary at the project root via
  `go build -o portal ./cmd/portal`; rename / copy to
  `portal-linux-amd64` before `docker build`
- Build args: `docker build --build-arg BINARY=portal --build-arg
  TARGETOS=linux --build-arg TARGETARCH=amd64 -t jamsesh/portal:e2e .`
- The portal image must include the SPA assets — verify the
  `frontend-build` step has run (it's a dependency of `go-build`)
- The image's `/healthz` is the only endpoint that responds usefully
  without further config; that's enough for this story's acceptance
- Add a guard in the test: if `docker info` fails, `t.Skip("docker
  not available")` — the same pattern is used elsewhere in the
  codebase for `requireGit()`
