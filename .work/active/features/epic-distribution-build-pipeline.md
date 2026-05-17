---
id: epic-distribution-build-pipeline
kind: feature
stage: implementing
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

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Workflow trigger surface**: tag push matching `v*` for the real
  signed release, plus `workflow_dispatch` for unsigned dry-runs.
  Manual dispatch produces artifacts as a workflow run output (no
  release, no signing, no marketplace push) — useful for testing
  changes to the workflow itself.
- **Single workflow file vs multiple**: one `.github/workflows/release.yml`.
  Splitting "build" and "sign" into separate workflows fragments the
  artifact passing (would require `actions/upload-artifact` + a
  download in a follow-on workflow); keeping it monolithic preserves
  the matrix→sign→publish data flow in-job-graph.
- **Go version pinning**: explicit `go-version: 1.22.x` in
  `actions/setup-go` (matched against `go.mod`'s `go 1.22` directive).
  Update in lockstep with `go.mod` toolchain changes.
- **Matrix shape**: a single matrix with both `binary` and `target`
  dimensions (10 jobs). One step per dimension keeps logs readable.
- **Cosign action**: `sigstore/cosign-installer@v3` then
  `cosign sign-blob --yes` per artifact (the recent best-practice path
  — `cosign-action` is deprecated; see `sigstore-cosign` skill).
- **SLSA provenance**: official `actions/attest-build-provenance@v1`
  applied to the consolidated artifact set after the matrix completes.
- **SBOM tool**: `anchore/sbom-action@v0` (Syft wrapper) — emits
  SPDX JSON per binary, attached to the release.
- **Release upload**: `softprops/action-gh-release@v2` (well-maintained,
  supports asset uploads + body composition).
- **Marketplace push**: NOT in this feature's scope. The `marketplace`
  feature consumes this workflow's artifacts via
  `actions/download-artifact` in its own workflow OR via a separate
  job within the same workflow gated behind a marketplace-only step
  — that decision is deferred to the marketplace feature's design pass.
- **Version injection**: `-X jamsesh/internal/buildinfo.Version=<tag>`
  for both binaries. The `buildinfo` package is one tiny Go file
  added by this story (it's small enough that splitting it into a
  separate story would be ceremony for ceremony's sake).

## Architectural choice

**Single tag-triggered workflow with a matrix build step, a fan-in
signing step, and a release-publish step.** Conceptual flow:

```
on: { push: { tags: [ 'v*' ] }, workflow_dispatch: {} }

jobs:
  build:
    strategy:
      matrix:
        binary: [portal, jamsesh]
        target:
          - { goos: linux,   goarch: amd64 }
          - { goos: linux,   goarch: arm64 }
          - { goos: darwin,  goarch: amd64 }
          - { goos: darwin,  goarch: arm64 }
          - { goos: windows, goarch: amd64 }
    steps:
      - checkout
      - setup-go (1.22.x, pinned)
      - go build with reproducibility flags + version inject
      - upload-artifact (binary)

  sign-and-release:
    needs: build
    if: startsWith(github.ref, 'refs/tags/')
    permissions:
      id-token: write     # for cosign keyless + SLSA provenance
      contents: write     # for gh release upload
      attestations: write # for attest-build-provenance
    steps:
      - download-artifact (all binaries)
      - sigstore/cosign-installer
      - cosign sign-blob (per artifact -> .sig + .pem)
      - anchore/sbom-action (per binary -> SPDX JSON)
      - generate checksums.txt (sha256 of every artifact)
      - cosign sign-blob checksums.txt
      - actions/attest-build-provenance (subject = all binaries)
      - softprops/action-gh-release (upload all .{bin,sig,pem,spdx.json},
        checksums.txt, checksums.txt.sig, checksums.txt.pem,
        attestation bundle)
```

Alternatives considered:

- **Reusable workflows (`workflow_call`)**: overhead for one consumer;
  no win.
- **goreleaser**: bundles a lot of this, but obscures the cosign +
  attestation wiring and adds a config file we'd need to keep in sync
  with our matrix. The locked decision in the epic is "do it
  ourselves" because the moving parts are small and the result is
  easier to audit.

## Implementation Units

### Unit 1: Release workflow

**File**: `.github/workflows/release.yml`
**Story**: `epic-distribution-build-pipeline-release-workflow`

Annotated sketch (final YAML in implementation):

```yaml
name: release
on:
  push:
    tags: ['v*']
  workflow_dispatch:
    inputs:
      dry_run:
        description: 'Skip signing / release upload'
        type: boolean
        default: true

permissions: read-all  # locked-down default; jobs request more

env:
  GO_VERSION: '1.22.x'

jobs:
  build:
    name: build (${{ matrix.binary }} ${{ matrix.target.goos }}/${{ matrix.target.goarch }})
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false
      matrix:
        binary: [portal, jamsesh]
        target:
          - { goos: linux,   goarch: amd64 }
          - { goos: linux,   goarch: arm64 }
          - { goos: darwin,  goarch: amd64 }
          - { goos: darwin,  goarch: arm64 }
          - { goos: windows, goarch: amd64 }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          check-latest: false
          cache: true
      - name: build
        env:
          GOOS: ${{ matrix.target.goos }}
          GOARCH: ${{ matrix.target.goarch }}
          CGO_ENABLED: '0'
        run: |
          set -euo pipefail
          tag="${GITHUB_REF_NAME}"
          if [ "${tag#v}" = "${tag}" ]; then
            # workflow_dispatch case; manufacture a dev version
            tag="0.0.0-dev-$(date -u +%Y%m%dT%H%M%SZ)-${GITHUB_SHA::8}"
          fi
          ext=''
          if [ "${GOOS}" = 'windows' ]; then ext='.exe'; fi
          out="dist/${{ matrix.binary }}-${GOOS}-${GOARCH}${ext}"
          mkdir -p dist
          go build \
            -trimpath \
            -buildvcs=false \
            -ldflags "-s -w -X jamsesh/internal/buildinfo.Version=${tag} -X jamsesh/internal/buildinfo.Commit=${GITHUB_SHA}" \
            -o "${out}" \
            ./cmd/${{ matrix.binary }}
      - uses: actions/upload-artifact@v4
        with:
          name: ${{ matrix.binary }}-${{ matrix.target.goos }}-${{ matrix.target.goarch }}
          path: dist/*
          if-no-files-found: error
          retention-days: 7

  sign-and-release:
    name: sign and publish
    runs-on: ubuntu-latest
    needs: build
    if: github.event_name == 'push' && startsWith(github.ref, 'refs/tags/v')
    permissions:
      id-token: write
      contents: write
      attestations: write
    steps:
      - uses: actions/checkout@v4
      - uses: actions/download-artifact@v4
        with:
          path: dist-staging
          merge-multiple: true
      - name: assemble dist/
        run: |
          mkdir -p dist
          mv dist-staging/* dist/
          ls -la dist/
      - uses: sigstore/cosign-installer@v3
      - name: cosign sign each artifact
        env:
          COSIGN_EXPERIMENTAL: '1'  # legacy guard; v3 doesn't require it
        run: |
          set -euo pipefail
          for f in dist/*; do
            cosign sign-blob --yes \
              --bundle "${f}.cosign.bundle" \
              --output-signature "${f}.sig" \
              --output-certificate "${f}.pem" \
              "${f}"
          done
      - name: generate sbom
        uses: anchore/sbom-action@v0
        with:
          path: dist
          format: spdx-json
          output-file: dist/sbom.spdx.json
      - name: checksums
        run: |
          set -euo pipefail
          cd dist
          # Exclude already-emitted signatures/certs/bundles/sbom from the
          # checksum file (we sign the binaries, not the sigs).
          find . -maxdepth 1 -type f \
            ! -name '*.sig' ! -name '*.pem' ! -name '*.cosign.bundle' \
            ! -name 'sbom.spdx.json' ! -name 'checksums*' \
            -printf '%f\n' | sort | xargs sha256sum > checksums.txt
          cosign sign-blob --yes \
            --bundle checksums.txt.cosign.bundle \
            --output-signature checksums.txt.sig \
            --output-certificate checksums.txt.pem \
            checksums.txt
      - name: build provenance attestation
        uses: actions/attest-build-provenance@v1
        with:
          subject-path: 'dist/*'
      - uses: softprops/action-gh-release@v2
        with:
          files: dist/*
          fail_on_unmatched_files: true
          generate_release_notes: true
```

**Implementation Notes**:
- The matrix produces uniform artifact names: `<binary>-<goos>-<goarch>[.exe]`.
  Downstream features (`docker-image`, `marketplace`) rely on this
  naming convention.
- `CGO_ENABLED=0` is critical for cross-compilation. The portal uses
  pure-Go SQLite (`modernc.org/sqlite`) and pure-Go Postgres
  (`pgx/v5`), so no cgo is required.
- The signing job is gated on real tag pushes only — `workflow_dispatch`
  produces unsigned artifacts (visible in workflow run UI, not
  released).
- Permissions are scoped per-job: `read-all` at workflow level;
  `sign-and-release` explicitly opts into `id-token: write` for
  Sigstore OIDC and SLSA, `contents: write` for release upload, and
  `attestations: write` for `attest-build-provenance`.

### Unit 2: Build-info Go package

**File**: `internal/buildinfo/buildinfo.go`
**Story**: `epic-distribution-build-pipeline-release-workflow`

```go
// Package buildinfo carries link-time-injected release identifiers.
// Values come from -ldflags="-X .../buildinfo.Version=... -X .../buildinfo.Commit=..."
// at build time. In dev (`go run`), they read as their compile-time
// defaults "dev" and "unknown".
package buildinfo

var (
    Version = "dev"
    Commit  = "unknown"
)

// String returns "version (commit)" — useful for /healthz response and
// `--version` flags as they're added.
func String() string {
    return Version + " (" + Commit + ")"
}
```

**Acceptance Criteria** (Units 1-2):
- [ ] `.github/workflows/release.yml` lints clean with
      `actionlint` (or equivalent)
- [ ] `workflow_dispatch` run produces 10 artifacts in
      `<binary>-<goos>-<goarch>[.exe]` form
- [ ] Tag push `v0.0.1-rc.0` produces a GitHub release with the 10
      binaries, 10 `.sig`/`.pem`/`.cosign.bundle` triples, one
      `sbom.spdx.json`, one `checksums.txt` + `.sig`/`.pem`/`.bundle`,
      and one build-provenance attestation file
- [ ] `cosign verify-blob` against any released binary succeeds with
      `--certificate-identity-regexp` pinned to the jamsesh
      `.github/workflows/release.yml` and
      `--certificate-oidc-issuer https://token.actions.githubusercontent.com`
      (procedure documented in self-host-docs, but the test target
      is asserted here)
- [ ] Build is reproducible: building the same tag twice (different
      runner) yields identical binary checksums for at least linux-
      amd64 (assertion-quality reproducibility for all 5 targets is
      a stretch goal — track gaps in feature body Risks)
- [ ] `internal/buildinfo` package compiles and is consumed by both
      `cmd/portal` and `cmd/jamsesh` (the latter is added by the CC
      plugin epic — until then, the workflow's plugin build job will
      fail; this is an expected sequencing gap, noted in the story)

## Implementation Order

Single story; everything in one stride.

## Testing

### Validation strategy

Real validation requires landed `cmd/portal` (http-skeleton) and
landed `cmd/jamsesh` (cc-plugin epic). Until both exist, the workflow
is staged-but-failing. Validation steps:

1. **Now (this story lands)**: lint workflow, validate YAML, dry-run
   any reusable composite actions.
2. **After http-skeleton config-tls-and-entry lands**: a
   `workflow_dispatch` run produces 5 portal binaries; the 5 jamsesh
   builds fail with "package not found" — expected.
3. **After cc-plugin-binary-foundation lands**: full matrix runs
   green on dispatch.
4. **First real tag (`v0.0.1-rc.0`)**: full pipeline runs; an external
   verifier checks `cosign verify-blob` against a downloaded artifact.

### Unit tests for `internal/buildinfo`

`internal/buildinfo/buildinfo_test.go` — assert defaults are non-empty
and `String()` round-trips.

## Risks

- **Reproducibility across runner image updates.** GitHub-hosted
  runner OS images change; even with `-trimpath` + `-buildvcs=false`
  + fixed Go version, link-time noise can sneak in (toolchain
  patch-version bumps, glibc inclusion). Mitigation: pin Go to
  exact patch when reproducibility regressions are observed;
  document the local-reproduction recipe in `docs/SELF_HOST.md`.
- **Sigstore-action API churn.** `sigstore/cosign-installer` is stable
  but the underlying cosign CLI flag surface evolves. Mitigation:
  pin the action major version (`@v3`); update with a tested-rollback
  recipe.
- **SLSA `attest-build-provenance` subject-path globbing.** Glob
  matching for `dist/*` includes sig files and bundles; the
  attestation should subject only the binary set. Mitigation: if
  the attestation includes too many subjects, switch
  `subject-path` to an explicit allow-list generated by a prior step.
- **Sequencing gap with cmd/portal and cmd/jamsesh.** The workflow
  is the foundation feature but the binaries it builds don't exist
  yet. Mitigation: workflow_dispatch + per-matrix `continue-on-error:
  true` could be a temporary measure but masks real failures —
  prefer "expected failures during sequencing" and a checklist note
  on the story.
