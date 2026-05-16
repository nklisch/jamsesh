---
id: epic-distribution
kind: epic
stage: drafting
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

## Anticipated child features

Provisional — actual decomposition lands when this epic is designed.

- Portal binary multi-arch builds (Linux/macOS/Windows × amd64/arm64)
- Plugin binary multi-arch builds (same matrix; per-platform `jamsesh`
  binary in marketplace package)
- Portal Docker image (multi-arch via buildx)
- CC marketplace repo setup (plugin manifest, structure, README)
- Plugin versioning + release process (semver bumps, changelog, tag-driven
  releases)
- Reproducible builds + Sigstore signing for all artifacts
- Self-host docs (install, config reference, TLS, backup, OAuth setup)

<!-- Design pass on each child feature will fill in specifics. -->
