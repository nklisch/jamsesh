---
id: epic-distribution
kind: epic
stage: done
tags: [infra]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Distribution

## Brief

Getting jamsesh into users' hands. Two distributable artifacts: the portal
binary (server-side) and the Claude Code plugin (client-side).

**Portal binary:** multi-architecture Go builds (Linux/macOS/Windows ×
amd64/arm64), Docker image as the convenience distribution. Configurable
via env vars or YAML config file (bind address, TLS certs, DB driver,
storage path, OAuth providers). Runs as systemd service, container, or
unmanaged process.

**CC plugin:** distributed via a GitHub-based marketplace repo per current
Claude Code plugin distribution model. The marketplace repo holds the
plugin manifest, per-platform `jamsesh` binaries in `bin/` (multi-arch),
skills, hooks, and MCP config. Plugin versioning is explicit semver
(`version` field in plugin.json) so updates are deterministic.

**Build infrastructure:** reproducible builds (Go's `-trimpath`,
`-buildvcs=false` for inputs determinism), release signing (Sigstore or
equivalent), checksums published with artifacts.

**Self-host docs:** README in the binary release explaining install,
configuration, and operational concerns (TLS termination, backup, OAuth
callback URL setup).

This epic is intentionally orthogonal — it can start as soon as there's
any portal-binary or plugin-package skeleton to wrap. It runs in parallel
to the implementation-arc epics.

This epic does NOT cover hosted-SaaS infrastructure (out of scope for v1
per VISION.md self-host-first stance).

## Foundation references

- `docs/SPEC.md` — Deployment shape, What's explicitly deferred
- `docs/SECURITY.md` — Supply chain and integrity, Self-host security
  posture

## Design decisions

- **CI/release pipeline**: GitHub Actions. Mature, free for public repos,
  matrix builds for multi-arch, native Sigstore action, fits the
  GitHub-based marketplace model.
- **Versioning**: synchronized. Plugin and portal both ship as the same
  semver (v0.4.2 etc.). Every portal release ships a matching plugin
  release. Costs a plugin bump when only portal changed; buys a much
  simpler mental model and guaranteed compat.
- **License**: Apache 2.0. Permissive with explicit patent grant; standard
  for Go infrastructure; maximum adoption headroom. Includes copyright
  headers + NOTICE file in the source tree.
- **First-public-release branding**: v0.1.0 with semver from day one.
  Pre-1.0 versions signal expect-breakage; bump 0.x.0 on breaking
  changes, 0.0.x for additive/fixes. Path to 1.0 when the API surface
  stabilizes. Apache-2.0-licensed source, multi-arch binaries, Docker
  image, Sigstore signatures, checksums published with every tagged release.

Locked at epic-design time (this pass):

- **Sigstore approach**: keyless OIDC via the GitHub Actions workflow's
  OIDC token. No key management; trust anchor is the workflow identity.
  Standard practice for OSS Go projects in 2026.
- **Container registry**: GHCR only for v0.x. Free for public repos,
  integrated with GitHub Actions and the marketplace. Docker Hub adds
  operational overhead with limited benefit (users can pull from GHCR
  directly).
- **Marketplace repo shape**: separate repo (`jamsesh-cc-plugin` or
  equivalent) per CC's marketplace convention. Source lives in the main
  `jamsesh` repo (under `epic-cc-plugin-packaging`'s scope); the release
  pipeline pushes built plugin artifacts to the marketplace repo on
  every tag.
- **Supply-chain attestations**: SLSA Build Level 3 via GitHub-hosted
  runners' provenance attestation + SBOM via Syft. Both off-the-shelf
  actions; minimal incremental cost; baseline supply-chain expectation
  for 2026 OSS.
- **Self-host docs location**: `README.md` (quick-start) + `docs/
  SELF_HOST.md` (full operator guide).

## Decomposition

Four child features. `build-pipeline` is the foundation — every other
release artifact derives from its outputs. `docker-image` and
`marketplace` are sibling consumers (different distribution channels).
`self-host-docs` is independent — it's documentation, sequenced
whenever the portal binary is buildable.

Critical path: `build-pipeline → {docker-image || marketplace}`, with
`self-host-docs` running in parallel.

### Child features

- `epic-distribution-build-pipeline` — GitHub Actions tag-triggered
  release workflow, multi-arch matrix (5 targets × 2 binaries),
  reproducible flags, Sigstore signing, SLSA + SBOM attestations,
  release artifact attaching — depends on: `[]`
- `epic-distribution-docker-image` — multi-arch (linux/amd64 + arm64)
  Docker image from distroless base, COPYs portal binary from
  build-pipeline output, pushes to GHCR with Sigstore signature —
  depends on: `[epic-distribution-build-pipeline]`
- `epic-distribution-marketplace` — `jamsesh-cc-plugin` GitHub repo
  populated by the release pipeline on every tag (manifest, per-arch
  plugin binaries, skills, hooks, .mcp.json) — depends on:
  `[epic-distribution-build-pipeline]`
- `epic-distribution-self-host-docs` — `README.md` + `docs/SELF_HOST.md`
  with install / config / TLS / OAuth / DB / email / backup /
  monitoring / upgrade / security / troubleshooting sections —
  depends on: `[]`

### Decomposition risks

- **Reproducibility across runner image updates.** Hosted-runner OS
  images change periodically; reproducible-build guarantees depend on
  pinned Go version + `-trimpath`. Mitigation: pin Go version
  explicitly in the workflow YAML; document local reproduction.
- **Sigstore OIDC trust anchor.** Keyless OIDC ties signature trust to
  the GitHub Actions workflow identity. A compromise of the main
  branch's workflow file could yield trusted-signed malicious builds.
  Mitigation: document the trust assumption in SELF_HOST.md and
  SECURITY.md; require code review on workflow changes via branch
  protection (operator concern, flagged for awareness).
- **CC marketplace conventions are evolving.** The manifest format
  and discovery mechanics could change. Mitigation: keep publishing
  tooling lightweight and re-runnable.
- **Self-host docs drift.** Operators rely on these for production
  setups; config-flag changes without doc updates cause real outages.
  Mitigation: tested-quickstart CI job keeps the install steps honest;
  the gate-docs skill at release-deploy time catches drift.

## Final review (2026-05-17)

**Verdict**: Approve

**Notes**: All 4 child features done: build-pipeline (release.yml with 10-target matrix + cosign + SLSA + SBOM), self-host-docs (README + SELF_HOST.md + quickstart-ci), docker-image (distroless + ghcr), marketplace (publish workflow). Six epics now at done.
