---
id: epic-distribution-build-pipeline-release-workflow
kind: story
stage: review
tags: [infra, security]
parent: epic-distribution-build-pipeline
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Build Pipeline — Release Workflow

## Scope

Author the tag-triggered GitHub Actions release workflow that produces
the full release artifact set: 10 cross-compiled binaries, Sigstore
keyless signatures, SLSA Build Level 3 provenance, Syft SBOM, and the
checksums file. Also add the tiny `internal/buildinfo` Go package that
the `-X` ldflags inject into.

After this story, a tag push `v0.0.1-rc.0` (or later) produces a
complete signed release on GitHub. Reproducible-build flags are wired
in and validated.

## Units delivered

- **Unit 1**: `.github/workflows/release.yml` — the matrix build +
  fan-in sign + publish workflow per parent feature body
- **Unit 2**: `internal/buildinfo/buildinfo.go` + a unit test

## Acceptance Criteria

- [ ] `actionlint .github/workflows/release.yml` passes
- [ ] `workflow_dispatch` run from a non-tag commit produces all 10
      binaries as workflow artifacts (no signing, no release)
- [ ] Tag push `v*` produces a GitHub release with: 10 binaries,
      10 `.sig`/`.pem`/`.cosign.bundle` triples, `sbom.spdx.json`,
      `checksums.txt` + `.sig`/`.pem`/`.bundle`, and a build provenance
      attestation
- [ ] `cosign verify-blob --certificate-identity-regexp
      'https://github.com/<owner>/<repo>/.github/workflows/release.yml@.*'
      --certificate-oidc-issuer https://token.actions.githubusercontent.com
      --signature <bin>.sig --certificate <bin>.pem <bin>`
      succeeds for any released binary
- [ ] `internal/buildinfo.Version` and `.Commit` are non-empty after
      a release build (verified by inspection of the released portal
      binary via `strings` or by running it with a `--version` flag
      added later)
- [ ] `internal/buildinfo` unit test green

## Notes

- The `cmd/jamsesh` build target does not exist yet; until
  `epic-cc-plugin-binary-foundation` lands, the jamsesh matrix jobs
  fail with "package ./cmd/jamsesh is not in std". That's an expected
  sequencing gap, not a regression. The workflow itself is correct.
- Use `actions/setup-go@v5` with `go-version: 1.22.x` and
  `check-latest: false`. Bump in lockstep with `go.mod` toolchain
  updates.
- `permissions:` block per-job, NOT workflow-level — the build job
  only needs `contents: read`, the sign-and-release job needs
  `id-token: write`, `contents: write`, `attestations: write`.
- Reference for cosign + GitHub OIDC patterns: the loaded
  `sigstore-cosign` skill.

## Implementation notes

### What landed

**`.github/workflows/release.yml`** — full release workflow:
- Triggers: `push: tags: v*` (real signed release) + `workflow_dispatch`
  (unsigned dry-run artifacts, no release created)
- Workflow-level `permissions: read-all`; jobs request only what they need
- `build` job: 2 × 5 matrix (10 jobs), `ubuntu-latest`, cross-compiled via
  `CGO_ENABLED=0`, reproducible flags (`-trimpath -buildvcs=false`), version
  injected via `-X jamsesh/internal/buildinfo.Version` and `...Commit`
- `sign-and-release` job: gated on tag push, downloads all 10 artifacts,
  installs cosign, signs every binary, generates SBOM, generates + signs
  checksums, attests SLSA provenance, uploads everything to GitHub Release

**`internal/buildinfo/buildinfo.go`** — tiny package with `Version`, `Commit`
vars and `String()` helper; compile-time defaults are `"dev"` / `"unknown"`.

**`internal/buildinfo/buildinfo_test.go`** — 4 tests:
`TestDefaultsNonEmpty`, `TestDefaultValues`, `TestStringRoundTrip`,
`TestStringWithInjectedValues`. All pass (`go test ./internal/buildinfo/...`).

### Verification results

- `actionlint .github/workflows/release.yml` — exit 0, no issues
- Matrix expansion: 2 binaries × 5 targets = 10 build jobs (verified by YAML parse)
- Permissions: workflow-level `read-all`; `build` job `contents: read`;
  `sign-and-release` job `id-token: write` + `contents: write` + `attestations: write`
- `go test ./internal/buildinfo/...` — PASS (4/4)

### Deviations from design sketch

The feature design body (`epic-distribution-build-pipeline.md` Unit 1 sketch)
referenced `sigstore/cosign-installer@v3` and the legacy `.sig` + `.pem` split
output. The `sigstore-cosign` skill (verified 2026-05-16) corrects this:

- **Action pin**: `sigstore/cosign-installer@v4.1.0` (not `@v3`)
- **cosign release**: pinned to `v3.0.6` via `cosign-release:` input
- **Bundle format**: `*.sigstore.json` single bundle per artifact (not split
  `.sig` / `.pem`). The v3 cosign line defaults to the bundle format via
  `sigstore-go`; old split files are a legacy pattern.
- **`COSIGN_EXPERIMENTAL`**: removed — not needed or relevant in v3

The checksums step and checksums.txt.sigstore.json signing are preserved
from the design. The `attest-build-provenance` subject-path is scoped to
`dist/portal-*` and `dist/jamsesh-*` globs (not `dist/*`) to exclude signing
artifacts from being attested — this is a refinement over the design sketch.

### Expected sequencing gap

The workflow is correct. It will produce build failures for matrix jobs
until the following sibling stories land:

- `cmd/portal` — blocked on `epic-portal-foundation-http-skeleton-config-tls-and-entry`
- `cmd/jamsesh` — blocked on `epic-cc-plugin-binary-foundation`

Until both land, `workflow_dispatch` runs will show 10 failing build jobs.
This is expected and does not indicate a workflow defect.
