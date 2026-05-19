# Releasing jamsesh

This is the maintainer reference for cutting a release. Self-hosters and end
users do not need this document — see [SELF_HOST.md](SELF_HOST.md) and the
[README quickstart](../README.md) instead.

---

## Overview

Releases are git tags of the form `vMAJOR.MINOR.PATCH` (e.g. `v0.1.0`). Pushing
a `v*` tag to GitHub triggers `.github/workflows/release.yml`, which:

1. Cross-compiles `portal` and `jamsesh` binaries for 5 OS/arch combos
   (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64) —
   10 binaries total.
2. Signs every binary with cosign keyless OIDC (Fulcio + Rekor), producing
   `<binary>.sigstore.json` bundles.
3. Generates an SPDX SBOM covering the `dist/` tree.
4. Generates and signs a `checksums.txt` over the binaries.
5. Attests SLSA Build Level 3 provenance over the binary subjects.
6. Creates a GitHub Release tagged `v<version>` with all artifacts attached.
7. Builds and pushes the multi-arch `ghcr.io/nklisch/jamsesh` Docker image
   with semver tags + `latest`, signed via cosign.
8. Plugin install: users install from `nklisch/jamsesh` directly. The
   `bin/jamsesh` wrapper script in the plugin fetches the matching binary
   from the release's GitHub assets on first run, verifies via sha256 +
   optional cosign, and caches under `${CLAUDE_PLUGIN_DATA}/bin/`.

The substrate's release-deploy skill (`/agile-workflow:release-deploy`)
handles steps the maintainer takes locally before pushing the tag —
binding items to the release, running the quality gates, drafting the
CHANGELOG entry. That skill is documented in
`.work/CONVENTIONS.md`.

---

## Cutting a release

1. **Drain the queue.** Every item bound to the upcoming release must be at
   `stage: done` before the tag pushes. Run
   `/agile-workflow:release-deploy <version>` to bind items, run gates,
   and walk to the readiness halt.

2. **Bump the compose template's `JAMSESH_VERSION` pin.** The self-host
   quickstart template at `deploy/compose/.env.example` pins
   `JAMSESH_VERSION` to a specific tag for reproducible operator deploys.
   Bump it to the version you're about to release, then commit:

   ```bash
   sed -i 's/^JAMSESH_VERSION=.*/JAMSESH_VERSION=v0.X.0/' deploy/compose/.env.example
   git add deploy/compose/.env.example
   git commit -m "release-prep: bump compose template to v0.X.0"
   ```

   Skipping this step means freshly-cloned operator setups will deploy
   the previous release until they edit their `.env` manually.

3. **Bump `bin/jamsesh` `JAMSESH_PLUGIN_VERSION`.** The plugin wrapper pins
   to a specific release tag for reproducible plugin installs. Bump before
   pushing:

   ```bash
   sed -i 's/^readonly JAMSESH_PLUGIN_VERSION=.*/readonly JAMSESH_PLUGIN_VERSION="v0.X.0"/' bin/jamsesh
   git add bin/jamsesh
   git commit -m "release-prep: bump plugin wrapper to v0.X.0"
   ```

   The release workflow asserts this matches the pushed tag and fails fast
   if it doesn't.

4. **Confirm the CHANGELOG entry.** `release-deploy` drafts `CHANGELOG.md`
   from the bound items + their commits. Review and edit before approving.

5. **Push the tag.** `release-deploy`'s ship phase tags and pushes
   `v<version>` to `origin`, which triggers `release.yml`. If you're
   doing this manually:

   ```bash
   git tag -a v0.X.0 -m "Release v0.X.0"
   git push origin main          # ensure remote has the commits the tag points at
   git push origin v0.X.0        # triggers the release workflow
   ```

6. **Watch the workflow.** Real-time view:

   ```bash
   gh run watch
   ```

   Or list recent runs of the release workflow:

   ```bash
   gh run list --workflow=release.yml --limit 5
   ```

7. **Verify the release.** Once `release.yml` finishes:

   ```bash
   gh release view v0.X.0
   ```

   The output lists every artifact uploaded, including the `.sigstore.json`
   signature bundles for each binary, the SBOM, and the signed
   `checksums.txt`.

---

## Verifying release signatures (for users)

Self-hosters verifying a downloaded binary should follow the cosign
instructions in [SELF_HOST.md](SELF_HOST.md). Maintainers should
manually verify at least one binary per release before announcing it:

```bash
cosign verify-blob \
  --bundle portal-linux-amd64.sigstore.json \
  --certificate-identity-regexp 'https://github.com/nklisch/jamsesh/.github/workflows/release.yml@refs/tags/v0.X.0' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  portal-linux-amd64
```

A green verification confirms the binary was built by `release.yml` on a
tag push and signed by the Fulcio+Rekor keyless chain.
