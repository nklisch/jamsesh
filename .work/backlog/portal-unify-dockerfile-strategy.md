---
id: portal-unify-dockerfile-strategy
kind: story
stage: drafting
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

## Acceptance criteria

- [ ] Single source of truth for the runtime image's package
      installations
- [ ] e2e tests continue to pass against whatever the unified strategy
      produces
- [ ] Both production deployment and the e2e test pipeline use the
      consistent image
