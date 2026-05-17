---
id: epic-distribution-build-pipeline-release-workflow
kind: story
stage: implementing
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
