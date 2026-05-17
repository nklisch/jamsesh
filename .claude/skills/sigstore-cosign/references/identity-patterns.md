# Certificate identity patterns for GitHub Actions keyless signing

The Fulcio-issued cert binds the signature to the workflow that produced
it. Identity strings look like:

```
https://github.com/<owner>/<repo>/.github/workflows/<file>@<ref>
```

`<ref>` is whatever ref triggered the workflow — a branch ref like
`refs/heads/main`, a tag ref like `refs/tags/v0.1.0`, or a SHA.

## jamsesh patterns

### Tagged release from the main branch

```
https://github.com/<org>/jamsesh/.github/workflows/release.yml@refs/tags/v0.1.0
```

Regex form (matches all tagged releases):

```
^https://github\.com/<org>/jamsesh/\.github/workflows/release\.ya?ml@refs/tags/v.*$
```

### Pre-release (RC) tags

If `v0.1.0-rc.1` style is used, the same regex catches them (`.*` after `v`).
Tighten with `v\d+\.\d+\.\d+(-[a-z0-9.]+)?` if RC trust differs from GA trust.

### Reusable workflow

If `release.yml` calls a reusable workflow elsewhere, the certificate
identity reflects the *reusable* workflow's path:

```
https://github.com/<owner>/<repo>/.github/workflows/<reusable>.yml@<ref>
```

Choose explicitly which one to pin — the *signer* identity is the
reusable workflow, not the caller. Document both for self-host
operators so they know exactly what they're trusting.

## Wildcard pitfalls

- `https://github.com/.*` is **too permissive** — any GH workflow can
  sign and pass verification.
- Always anchor `<org>` and `<repo>` (or pin the repo via
  `--certificate-github-workflow-repository`).
- `@refs/.*` matches every branch and tag — usually too loose. Pin to
  `@refs/tags/.*` for release verification.

## Sanity-check command

```bash
cosign verify-blob \
  --bundle <artifact>.sigstore.json \
  --certificate-identity-regexp '<your-regex>' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --offline \
  <artifact>
```

The `--offline` flag skips the Rekor lookup, useful for testing pure
bundle verification without network.
