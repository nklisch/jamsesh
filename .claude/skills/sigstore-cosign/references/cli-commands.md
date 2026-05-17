# cosign CLI quick reference (v3.0.6)

## sign-blob

```
cosign sign-blob [flags] <artifact>
```

Common flags for jamsesh keyless flow:

| Flag | Value | Why |
|------|-------|-----|
| `--yes` | (none) | Skip interactive confirmation; required in CI |
| `--bundle <path>` | `<artifact>.sigstore.json` | Emit single bundle file (v3 default) |
| `--oidc-issuer <url>` | (omit; auto-detected) | Picks up GH Actions issuer automatically when `id-token: write` is set |
| `--identity-token <jwt>` | (omit) | Auto-detected via `ACTIONS_ID_TOKEN_REQUEST_*` env vars in GHA |

Flags to avoid (legacy split-file output):

- `--output-signature <file>`
- `--output-certificate <file>`
- `--output-pubkey <file>`

## verify-blob

```
cosign verify-blob [flags] <artifact>
```

Required flags for a bundle-signed jamsesh artifact:

| Flag | Value | Why |
|------|-------|-----|
| `--bundle <path>` | `<artifact>.sigstore.json` | Verification material |
| `--certificate-identity-regexp <re>` | workflow URL regex | Pin trust anchor |
| `--certificate-oidc-issuer <url>` | `https://token.actions.githubusercontent.com` | GH Actions issuer |

Optional but recommended:

| Flag | Value | Why |
|------|-------|-----|
| `--certificate-github-workflow-repository <repo>` | `<org>/jamsesh` | Extra pin on the repo |
| `--certificate-github-workflow-ref <ref>` | `refs/tags/v*` | Pin to tagged refs only |

## Sign a container image (GHCR)

```
cosign sign --yes ghcr.io/<org>/jamsesh-portal:v0.1.0
```

The signature lives in OCI as a sibling tag; no extra files.

## Verify a container image

```
cosign verify ghcr.io/<org>/jamsesh-portal:v0.1.0 \
  --certificate-identity-regexp \
    'https://github.com/<org>/jamsesh/\.github/workflows/release\.ya?ml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Tag a sigstore release for reproducibility

In CI workflows, pin both the action and the cosign binary:

```yaml
- uses: sigstore/cosign-installer@v4.1.0   # or a SHA
  with:
    cosign-release: "v3.0.6"
```

## Useful subcommands beyond sign / verify

- `cosign tree <image>` — list signatures attached to an OCI image
- `cosign download signature <image>` — fetch raw signature material
- `cosign attest --type slsaprovenance ...` — attach an attestation
