---
name: sigstore-cosign
description: cosign keyless OIDC signing reference for jamsesh release artifacts (portal binary + per-platform CC plugin binaries). Auto-load when working with GitHub Actions release workflows, signing or verifying binaries, configuring `sigstore/cosign-installer`, writing verification docs for self-hosters, or updating supply-chain integrity claims. Triggers on `cosign`, `sigstore`, `keyless signing`, `OIDC`, `sign-blob`, `verify-blob`, `cosign-installer`.
user-invocable: false
---

# Sigstore cosign (jamsesh)

Keyless OIDC signing for the portal binary and the per-platform
`jamsesh` CC plugin binaries. Apache-2.0 release artifacts, signed via
GitHub Actions' OIDC token, verifiable offline by self-hosters and the
CC marketplace install path.

## Version pins (verified 2026-05-16)

- `cosign` CLI: **v3.0.6** (released 2026-04-06)
- `sigstore/cosign-installer` action: **`@v4.1.0`** or pin by SHA
- Bundle format: `*.sigstore.json` (v3 default; replaces `.sig` + `.pem`)
- GH Actions OIDC issuer: `https://token.actions.githubusercontent.com`

cosign v3 line defaults to bundles and uses `sigstore-go`'s signing
APIs under the hood. Do not emit the legacy split files.

## Keyless flow in one paragraph

The GitHub Actions runner has access to a per-job OIDC token (gated by
`permissions: id-token: write`). `cosign sign-blob` exchanges that token
with Fulcio for a short-lived (~10 minute) signing certificate bound to
the workflow identity, signs the artifact, uploads the signature entry
to Rekor (the public transparency log), and writes a single
`.sigstore.json` bundle containing the cert + signature + inclusion
proof. No long-lived private keys exist anywhere.

## Sign step (GitHub Actions)

```yaml
jobs:
  build-and-sign:
    runs-on: ubuntu-latest
    permissions:
      contents: write     # for release upload
      id-token: write     # REQUIRED for keyless OIDC
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Build (reproducible flags)
        run: |
          mkdir -p dist
          GOOS=linux GOARCH=amd64 go build \
            -trimpath -buildvcs=false \
            -o dist/jamsesh-portal-linux-amd64 ./cmd/portal

      - name: Install cosign
        uses: sigstore/cosign-installer@v4.1.0
        with:
          cosign-release: "v3.0.6"

      - name: Sign binary (keyless)
        run: |
          cosign sign-blob --yes \
            --bundle dist/jamsesh-portal-linux-amd64.sigstore.json \
            dist/jamsesh-portal-linux-amd64

      - name: Upload to release
        uses: softprops/action-gh-release@v2
        with:
          files: |
            dist/jamsesh-portal-linux-amd64
            dist/jamsesh-portal-linux-amd64.sigstore.json
```

The exact same step works for each `jamsesh` plugin binary in the
release matrix — `sign-blob` is artifact-agnostic. Five-platform plugin
matrix × one bundle each = five extra signing steps, all keyless.

## Verify step (operator-facing)

Document this in `docs/SELF_HOST.md`:

```bash
cosign verify-blob \
  --bundle jamsesh-portal-linux-amd64.sigstore.json \
  --certificate-identity-regexp \
    'https://github.com/<org>/jamsesh/\.github/workflows/release\.ya?ml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  jamsesh-portal-linux-amd64
```

The `certificate-identity-regexp` value pins the trust anchor to
*"signed by the release.yml workflow on a tagged commit of the jamsesh
repo"*. Anything else (a fork's workflow, a non-tag ref) fails
verification.

For the CC plugin install path, the marketplace install script runs the
same `cosign verify-blob` per downloaded binary before placing it on
disk. Pin `cosign-release` in any download-and-verify shell snippet so
operators get a known-good binary.

## Trust anchor (what `certificate-identity` means)

Keyless signatures bind to a workflow identity, not a person or a key.
Specifically: the certificate carries the workflow file path + ref. So
trusting a jamsesh signature means trusting that:

1. The `.github/workflows/release.yml` file on a tagged commit of
   `<org>/jamsesh` produced the signature.
2. The branch-protection rules on that file prevent unreviewed changes.

Document both expectations in `SECURITY.md`. The branch-protection
piece is an operator concern but is part of the threat model.

## Common pitfalls

- **Missing `id-token: write` permission.** The OIDC token request
  silently returns empty without it; `sign-blob` then errors with
  "missing OIDC token" instead of something useful. Always declare the
  permission at the job level.
- **Forgetting `--yes`.** Without it, `sign-blob` opens an interactive
  prompt and hangs the runner until the job times out.
- **Pinning the wrong issuer.** `https://github.com/login/oauth` is for
  GitHub *web login* OAuth, NOT Actions. Actions OIDC tokens come from
  `https://token.actions.githubusercontent.com`. Mixing them up gives a
  cryptic "certificate verification failed".
- **Legacy split files.** Some old guides emit `.sig` + `.pem` via
  `--output-signature` / `--output-certificate`. The 2026 idiom is one
  `*.sigstore.json` bundle per artifact. Verifiers built against
  `sigstore-go` expect the bundle.
- **Major-version drift on cosign-installer.** Pin to `@v4.1.0` or a
  SHA. Past major bumps have changed bundle layout in
  backward-incompatible ways.
- **Reusable workflows.** If the release job is invoked via a reusable
  workflow, the certificate identity reflects the *reusable* workflow
  path, not the caller's. Document the actual identity that ships in
  the certs.

## jamsesh-specific decisions

- **Keyless only.** No long-lived keys, no key rotation, no Vault
  integration. Trust anchor is the workflow identity (decision logged
  in `epic-distribution.md`).
- **GHCR + GitHub Releases only for v0.x.** Both surfaces consume the
  same `.sigstore.json` bundles.
- **Bundle alongside the binary in the release assets.** Naming
  convention: `<binary-filename>.sigstore.json`. No additional manifest
  index.
- **SLSA + SBOM stay separate.** Cosign signs the binary; `actions/attest-build-provenance`
  emits the SLSA attestation; Syft emits the SBOM. Three orthogonal
  supply-chain artifacts per binary.
- **GoReleaser deferred.** The release flow (two binaries × N
  platforms) is hand-rolled GH Actions steps for clarity. Revisit if
  the artifact list grows materially.

## Reference

See `references/cli-commands.md` for the full `sign-blob` / `verify-blob`
flag matrix and `references/identity-patterns.md` for example
`certificate-identity-regexp` values for common GH Actions configurations.
