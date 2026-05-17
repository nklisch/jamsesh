---
id: epic-distribution-build-pipeline
kind: feature
stage: drafting
tags: [infra]
parent: epic-distribution
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Distribution — Build Pipeline

## Brief

The release CI/CD core. A GitHub Actions workflow (tag-triggered)
that produces every artifact for a tagged release: the portal binary
and the `jamsesh` plugin binary across all five targets (darwin-
amd64, darwin-arm64, linux-amd64, linux-arm64, windows-amd64), with
Sigstore-keyless signatures and SHA-256 checksums.

**Build matrix**: 5 targets × 2 binaries = 10 build jobs. Each job:

- Runs on `ubuntu-latest` cross-compiling via Go's GOOS/GOARCH (no
  matrix-runner-per-OS — cross-compile from one runner is simpler
  and more reproducible).
- Compiles with reproducibility flags: `-trimpath` (strip
  workspace path), `-buildvcs=false` (no VCS info in binary),
  `-ldflags="-s -w -X main.version=<tag>"` (strip + inject version).
- Pinned Go version in the workflow YAML (matches `go.mod` toolchain
  directive); never `latest`.

**Signing & attestations**:

- Sigstore keyless signing via `sigstore/cosign-action` using the
  workflow's OIDC token. Every artifact gets a `.sig` + `.pem`
  alongside the binary.
- SLSA Build Level 3 provenance attestation via the official
  `actions/attest-build-provenance` (auto-generated for runs on
  GitHub-hosted runners).
- SBOM generation via Syft for both binaries; attached to the
  release.
- `checksums.txt` containing SHA-256 of every artifact;
  countersigned by Sigstore.

**Release artifact attaching**: all artifacts uploaded to the GitHub
Release for the tag (via `softprops/action-gh-release` or
equivalent). Plugin binaries are also pushed to the marketplace repo
by the `marketplace` feature, which consumes from this feature's
artifacts.

**Manual triggers**: workflow also supports `workflow_dispatch` for
dry-runs without a tag (produces unsigned artifacts in a workflow
run, for testing).

Does NOT cover the Docker image (`docker-image` feature). Does NOT
cover the marketplace repo publishing (`marketplace` feature). Does
NOT cover docs (`self-host-docs`).

## Epic context

- Parent epic: `epic-distribution`
- Position in epic: foundation — every other feature in this epic
  consumes its outputs (binaries → docker-image and marketplace;
  checksums + signatures → all consumers).

## Foundation references

- `docs/SPEC.md` — Deployment shape, Stack (Go version
  considerations)
- `docs/SECURITY.md` — Supply chain and integrity (reproducible
  builds, release signing)

## Inherited epic design decisions

- **CI**: GitHub Actions.
- **Versioning**: synchronized portal + plugin (same semver per tag).
- **Sigstore approach**: keyless OIDC via GitHub Actions OIDC token.
  No key management; trust anchor is the workflow identity.
- **SLSA + SBOM**: Build Level 3 provenance + Syft-generated SBOM
  attached to every release.

## Decomposition risks

- **Reproducibility across runner image updates.** GitHub Actions
  hosted-runner images change periodically; reproducible-build
  guarantees depend on pinned Go version + `-trimpath`. Mitigation:
  pin Go version explicitly; document how to reproduce locally with
  the same Go version.
- **Sigstore OIDC trust anchor.** Keyless OIDC ties signature trust
  to the GitHub Actions workflow identity. A compromise of the main
  branch's workflow file could be leveraged for trusted-signed
  malicious builds. Mitigation: document the trust assumption in
  SELF_HOST.md and SECURITY.md; require code review on workflow
  changes via branch protection (operator concern, but flagged).

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
