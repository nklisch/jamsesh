---
id: portal-unify-dockerfile-strategy
kind: story
stage: review
tags: [infra, cleanup]
parent: null
depends_on: [portal-prod-dockerfile-base-image-review]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Unify production Dockerfile and Dockerfile.e2e

## Context

After commit `7501fdf` the repo has two Docker images:
- `Dockerfile` — production, debian:bookworm-slim + git +
  ca-certificates, USER nobody:nogroup
- `Dockerfile.e2e` — e2e tests, alpine:3.21 + git (no USER directive,
  so root)

Both install git the same way (different package manager); both copy
the same statically-linked binary. The duplication is a maintenance
hazard — changes to one might not be reflected in the other (e.g., if
production gains a new runtime dep, e2e tests will fail in a
non-obvious way).

## Suggested resolutions

After `portal-prod-dockerfile-base-image-review` lands, pick one of:

1. **Unify on the production base** — delete `Dockerfile.e2e`; the e2e
   test target uses the production Dockerfile. Pro: single source of
   truth. Con: e2e image is 80MB instead of 15MB.

2. **Multi-stage with shared base** — use Docker multi-stage so the
   production image and e2e image share an early stage. Pro: shared
   base layer cached locally. Con: more complex Dockerfile.

3. **Both with shared script** — extract the git+ca-cert install into
   a shared shell script invoked from both Dockerfiles. Pro: simple.
   Con: still two files.

## Decision

**Unify on the production Dockerfile (option 1).** Now that the
production base is alpine:3.21 + git + ca-certificates (see
`portal-prod-dockerfile-base-image-review`), the production image and
the e2e image are identical in every meaningful way — same base, same
runtime deps, same static binary. Keeping two Dockerfiles would just
duplicate the layer instructions for no reason.

Options 2 (multi-stage) and 3 (shared script) were both rejected:
they re-introduce indirection to solve a problem that no longer
exists once the base images match.

## Implementation notes

- `Dockerfile.e2e` deleted.
- `Dockerfile` is now the single source of truth. Includes
  `ca-certificates` (which `Dockerfile.e2e` lacked) so the unified
  image can talk to outbound TLS endpoints without surprises.
- `Makefile`'s `test-portal-image` target dropped `-f Dockerfile.e2e`
  and now uses the default `Dockerfile`. Comment updated to reflect
  that the e2e image IS the production image.

### Impact on in-flight work

The in-flight `portal-test-clock-advance-endpoint-test-endpoint`
child story (from `portal-test-clock-advance-endpoint`'s design)
references `Dockerfile.e2e` in its planned Makefile change. When that
story is picked up, the implementer should adapt to the unified
`Dockerfile` — passing `-tags e2etest` to `go build` at the Makefile
level rather than at `docker build` time still works since the
binary is built externally and `COPY`d in.

## Acceptance criteria

- [x] Single source of truth for the runtime image's package
      installations — alpine:3.21 + apk add git ca-certificates
- [x] e2e tests continue to pass against whatever the unified strategy
      produces — same base as before, plus ca-certificates; no
      behavior change to test
- [x] Both production deployment and the e2e test pipeline use the
      consistent image — `Makefile`'s `test-portal-image` now uses
      `Dockerfile`, same as the release pipeline
