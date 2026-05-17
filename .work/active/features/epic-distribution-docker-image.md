---
id: epic-distribution-docker-image
kind: feature
stage: done
tags: [infra]
parent: epic-distribution
depends_on: [epic-distribution-build-pipeline]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
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

## Design decisions

- **Dockerfile pattern**: simple COPY-from-artifact (NOT multi-stage build-from-source). The release workflow builds binaries, downloads them as artifacts, then `docker buildx build` per-arch with `--build-arg BINARY=<artifact-path>` copying the matching binary in. Distroless static base. Single Dockerfile, parameterized by buildx platform.
- **Workflow integration**: extend `.github/workflows/release.yml` with a `docker` job that runs after `build` and runs in parallel with `sign-and-release` (or sequentially — design says parallel is fine since each operates on different artifacts).
- **Tagging**: `${GHCR_REPO}:v${VERSION}`, `:v${MAJOR}.${MINOR}`, `:v${MAJOR}`, `:latest` (on non-prerelease).
- **Single story** — small surface.

## Implementation Units

### Unit 1: Dockerfile

**File**: `Dockerfile`

```dockerfile
ARG BINARY=portal
FROM gcr.io/distroless/static:nonroot
ARG TARGETOS
ARG TARGETARCH
COPY ${BINARY}-${TARGETOS}-${TARGETARCH} /usr/local/bin/portal
EXPOSE 8443
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/portal"]
```

### Unit 2: Release workflow extension

**File**: `.github/workflows/release.yml` (edit)

Add a `docker` job:

```yaml
docker:
  name: docker image
  runs-on: ubuntu-latest
  needs: build
  if: github.event_name == 'push' && startsWith(github.ref, 'refs/tags/v')
  permissions:
    contents: read
    packages: write
    id-token: write  # for cosign keyless
  steps:
    - uses: actions/checkout@v4
    - uses: actions/download-artifact@v4
      with:
        path: dist-staging
        merge-multiple: true
    - name: assemble binaries
      run: |
        mkdir -p docker-context
        cp dist-staging/portal-linux-amd64 docker-context/
        cp dist-staging/portal-linux-arm64 docker-context/
        cp Dockerfile docker-context/
    - uses: docker/setup-qemu-action@v3
    - uses: docker/setup-buildx-action@v3
    - uses: docker/login-action@v3
      with:
        registry: ghcr.io
        username: ${{ github.actor }}
        password: ${{ secrets.GITHUB_TOKEN }}
    - name: docker metadata
      id: meta
      uses: docker/metadata-action@v5
      with:
        images: ghcr.io/${{ github.repository_owner }}/jamsesh
        tags: |
          type=semver,pattern=v{{version}}
          type=semver,pattern=v{{major}}.{{minor}}
          type=semver,pattern=v{{major}}
          type=raw,value=latest,enable=${{ !contains(github.ref_name, '-') }}
    - uses: docker/build-push-action@v6
      id: build
      with:
        context: docker-context
        push: true
        platforms: linux/amd64,linux/arm64
        tags: ${{ steps.meta.outputs.tags }}
        labels: ${{ steps.meta.outputs.labels }}
    - uses: sigstore/cosign-installer@v4.1.0
      with:
        cosign-release: v3.0.6
    - name: cosign sign image
      run: |
        cosign sign --yes ghcr.io/${{ github.repository_owner }}/jamsesh@${{ steps.build.outputs.digest }}
```

## Testing

- `docker buildx build` locally with test binaries → image builds
- Workflow lint via `actionlint`
- First real tag triggers full build + push + sign chain

## Risks

- **Distroless base updates**: pin a specific distroless digest in v0.x.y to avoid surprises. Documented; not v1-blocking.
- **Multi-arch buildx setup**: depends on QEMU emulation. Standard pattern.

## Single Story

`epic-distribution-docker-image-dockerfile-and-workflow` — covers Dockerfile + workflow extension.

## Implementation summary

Single story done.

## Review

**Verdict**: Approve. Capability complete.
