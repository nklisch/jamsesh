---
id: epic-distribution-docker-image
kind: feature
stage: drafting
tags: [infra]
parent: epic-distribution
depends_on: [epic-distribution-build-pipeline]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Distribution — Docker Image

## Brief

Multi-arch Docker image for the portal binary, built and signed
alongside the binary release, pushed to GitHub Container Registry
(GHCR) under `ghcr.io/<org>/jamsesh-portal`. The convenience
distribution for operators who prefer containerized deployment.

**Image structure**:

- Base: a minimal distroless image (`gcr.io/distroless/static`) for
  the linux/amd64 + linux/arm64 manifests. No shell, no package
  manager — just the static portal binary.
- The binary is COPYed in from the `build-pipeline` feature's
  output for the matching architecture (rather than building from
  source in a multi-stage Dockerfile — one build per release,
  consistent binaries across deployment modes).
- ENTRYPOINT to the portal binary with default args.
- Exposes `8443` (HTTPS) by default.
- Reads config from `/etc/jamsesh/config.yaml` if present; env vars
  override.

**Build mechanics**:

- Dockerfile in repo at `Dockerfile`.
- Built via `docker/build-push-action` with `platforms:
  linux/amd64,linux/arm64` in the same release workflow as
  `build-pipeline` (or a sibling workflow that runs after the
  binaries are available).
- Tagged with the release version (`vX.Y.Z`), `vX.Y` (minor track),
  `vX` (major track), and `latest` on the highest non-prerelease.
- Signed via Sigstore keyless OIDC (same approach as binary
  signing).
- Pushed to GHCR; image digest published in the release notes.

**Registry**: GHCR only for v0.x (locked at epic-design — Docker Hub
adds operational overhead for limited benefit; GHCR is free for
public repos and integrated with the release workflow).

Does NOT cover the binary build itself (`build-pipeline`). Does NOT
cover image-side compose files / k8s manifests (out of scope for v0.x;
operators run the image however their infra prefers).

## Epic context

- Parent epic: `epic-distribution`
- Position in epic: secondary distribution channel; consumes
  `build-pipeline` output.

## Foundation references

- `docs/SPEC.md` — Deployment shape (Docker image as convenience
  distribution)
- `docs/SECURITY.md` — Supply chain and integrity

## Inherited epic design decisions

- **Registry**: GHCR only for v0.x.
- **Signing**: Sigstore keyless OIDC, same as binaries.
- **Image base**: distroless static (minimal surface).
- **Versioning**: matches the release tag.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
