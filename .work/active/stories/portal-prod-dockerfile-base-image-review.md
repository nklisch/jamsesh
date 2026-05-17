---
id: portal-prod-dockerfile-base-image-review
kind: story
stage: drafting
tags: [infra, security, documentation]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Production Dockerfile base image: ops review of distroless → debian switch

## Context

Commit `7501fdf` changed the production Dockerfile base from
`gcr.io/distroless/static:nonroot` to `debian:bookworm-slim` + apt-installed
git + ca-certificates. The change was made during e2e implementation
because the portal's `internal/portal/storage/repo.go` calls
`exec.Command("git", "init", "--bare")`, which fails in distroless (no
git binary). The previous production image was effectively broken — any
session creation that runs `git init` would have failed in production.

The fix is correct — the portal NEEDS git available at runtime. But the
base-image switch is a substantial operations change that warrants
explicit review:

## Concerns to evaluate

1. **Image size**: distroless/static is ~5MB; debian:bookworm-slim + git +
   ca-certificates is ~80MB. ~16× growth. Affects pull times, registry
   storage, deployment cost.

2. **Attack surface**: distroless ships only the binary + minimal libs.
   debian:bookworm-slim ships a full shell, package manager, system
   utilities. Larger CVE surface; needs ongoing security updates.

3. **Runtime user**: `USER nonroot:nonroot` → `USER nobody:nogroup`. This
   changes the UID/GID the portal runs as. Existing deployments that
   rely on a specific user (volume mounts, file ownership) may break.

4. **cosign signing**: `release.yml` signs the published binary with
   sigstore cosign. The base-image change shouldn't affect binary
   signing, but verify the release workflow still produces correctly
   signed artifacts.

5. **Alpine alternative**: `Dockerfile.e2e` already uses
   `alpine:3.21 + apk add git` (~15MB). Why not use Alpine for
   production too? Or wolfi-images for a hardened minimal git image.

## Suggested resolutions (pick one)

1. **Keep debian** — accept the size/surface trade-off; document the
   reasoning in `docs/SELF_HOST.md`.
2. **Switch to alpine** — match `Dockerfile.e2e`'s base for consistency;
   smaller and well-supported. Verify musl libc compatibility (the
   binary is `CGO_ENABLED=0` so it's static — no glibc dep — alpine
   should work).
3. **Use a hardened git base** — `cgr.dev/chainguard/git` or similar;
   smaller than debian, distroless-like security posture.

## Acceptance criteria

- [ ] Decision made on the base image (debian / alpine / hardened)
- [ ] `docs/SELF_HOST.md` updated to reflect the chosen image and any
      ops implications (volume permissions for the new user, image
      pull strategy)
- [ ] `release.yml` verified — produces signed artifacts on the new
      base
- [ ] If the user changed (UID/GID), migration notes for existing
      deployments are documented

## Notes

This finding surfaced during e2e implementation of
`epic-e2e-tests-golden-path-session-lifecycle`. The implementer
correctly fixed the latent production bug (distroless lacking git);
the ops-review-needed dimension is a separate concern.
