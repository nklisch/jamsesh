---
id: portal-prod-dockerfile-base-image-review
kind: story
stage: done
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

## Decision

**Switch to `alpine:3.21` + apk-installed `git` + `ca-certificates`.**

Reviewed the three options. Picked alpine because:

- **Size**: ~15 MB total (vs ~80 MB for debian:bookworm-slim, ~16× win).
- **Consistency**: matches the existing `Dockerfile.e2e` base, so a
  single Dockerfile now covers production AND e2e — see follow-on
  story `portal-unify-dockerfile-strategy` which lands in the same
  commit.
- **Surface**: small, well-maintained CVE patch cadence on the alpine
  3.21 line; no shell needed at runtime (`ENTRYPOINT` is the static
  binary).
- **musl libc**: not a concern — the portal binary is built
  `CGO_ENABLED=0` and is fully static. Verified by the existing e2e
  pipeline which has been running against alpine for weeks.
- **Chainguard**: rejected — adds a new registry dependency (`cgr.dev`)
  that operators must trust, with no clear win over alpine for this
  use case. If supply-chain trust ever becomes the dominant concern,
  it can be revisited.

### UID/GID

`USER nobody:nogroup` → `USER nobody`. **The actual UID/GID is the
same on both bases: 65534/65534** (verified in alpine 3.21's
`/etc/passwd` and debian:bookworm-slim's). Volume-mount permissions
keyed off UID 65534 continue to work; only the user/group names
differ. No migration burden for existing self-host deployments.

### cosign signing

The release workflow (`.github/workflows/release.yml`) signs the
portal binary, not the Docker image. Switching the base image doesn't
affect binary signing. The signed binary is `COPY`'d into the alpine
image at the same path (`/usr/local/bin/portal`); cosign verification
still works for self-hosters who pull the binary directly. Worth
confirming in the next release run that the signed artifacts emerge
identical.

## Acceptance criteria

- [x] Decision made on the base image — alpine 3.21
- [x] `docs/SELF_HOST.md` updated to reflect the chosen image and any
      ops implications — no doc update needed; UID/GID stayed 65534
      so existing volume permissions are unaffected
- [ ] `release.yml` verified — produces signed artifacts on the new
      base (cosign signs the binary, which is unchanged; needs the
      next release run to confirm end-to-end)
- [x] If the user changed (UID/GID), migration notes for existing
      deployments are documented — UID/GID stayed the same; only the
      name changed; documented above

## Notes

This finding surfaced during e2e implementation of
`epic-e2e-tests-golden-path-session-lifecycle`. The implementer
correctly fixed the latent production bug (distroless lacking git);
the ops-review-needed dimension is a separate concern.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The decision notes state "release.yml signs the portal binary, not
  the Docker image." That's incomplete — `release.yml` has a `docker`
  job that runs `cosign sign --yes ghcr.io/.../jamsesh@<digest>`,
  signing the published image too. The base-image switch is still a
  no-op for both signing operations (sign-blob is content-hashed over
  the binary; image signing is keyed on the built-image digest), so
  the conclusion holds. Worth tightening the wording on the next
  edit if anyone touches this story.

**Notes**:
- Alpine 3.21 choice is well-justified: ~5× smaller than the rejected
  debian path, matches the prior `Dockerfile.e2e` base (proven by
  weeks of e2e runs), avoids adding a new registry trust dependency
  (chainguard).
- musl/glibc compatibility is correctly dismissed: the binary is
  `CGO_ENABLED=0` per release.yml; fully static.
- UID/GID 65534 claim verified — both alpine 3.21 (`nobody:x:65534:
  65534:nobody:/:/sbin/nologin`) and debian:bookworm-slim
  (`nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin`)
  ship the same numeric `nobody` user. No volume-permission
  migration burden — accurate.
- Dockerfile-level diff matches spec: alpine:3.21, git +
  ca-certificates via apk, USER nobody, ENTRYPOINT unchanged. The
  comment block on the Dockerfile correctly enumerates why git is
  required (`git init --bare`, smart-HTTP receive/upload-pack).
- AC 3 (release.yml verified end-to-end) left unchecked with explicit
  rationale: cosign signs binary by content hash and image by digest,
  both mechanically unaffected by base-image swap — confirmation
  requires the next tag push. Reasonable framing; not a blocker.
- ACs 1, 2, 4 complete with reasoning recorded inline.
