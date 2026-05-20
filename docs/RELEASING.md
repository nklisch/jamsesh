# Releasing jamsesh

This is the maintainer reference for cutting a release. Self-hosters and end
users do not need this document — see [SELF_HOST.md](SELF_HOST.md) and the
[README quickstart](../README.md) instead.

---

## Overview

Releases are git tags of the form `vMAJOR.MINOR.PATCH` (e.g. `v0.1.0`). Pushing
a `v*` tag to GitHub triggers `.github/workflows/release.yml`, which:

1. Runs `make frontend-build` for the `portal` matrix entries (Node 20, npm
   cache) to populate the embedded Svelte SPA in `internal/portal/assets/dist/`
   before `go build` runs. The `jamsesh` wrapper entries skip this step.
2. Cross-compiles `portal` and `jamsesh` binaries for 5 OS/arch combos
   (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64) —
   10 binaries total.
3. Signs every binary with cosign keyless OIDC (Fulcio + Rekor), producing
   `<binary>.sigstore.json` bundles.
4. Generates an SPDX SBOM covering the `dist/` tree.
5. Generates and signs a `checksums.txt` over the binaries.
6. Attests SLSA Build Level 3 provenance over the binary subjects.
7. Creates a GitHub Release tagged `v<version>` with all artifacts attached.
8. Builds and pushes the multi-arch `ghcr.io/nklisch/jamsesh` Docker image
   with semver tags + `latest`, signed via cosign.
9. Plugin install: users install from `nklisch/jamsesh` directly. The
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

2. **Bump versions, tag, and push.** Run the release bump script from a
   clean `main` branch with the CHANGELOG already committed (step 1 above):

   ```bash
   scripts/release-bump.sh vX.Y.Z
   ```

   The script edits three files (`bin/jamsesh`, `deploy/compose/.env.example`,
   `.claude-plugin/plugin.json`), commits them as `release-prep: vX.Y.Z`,
   creates an annotated tag, and pushes `main` then the tag to `origin`.
   The tag push triggers `release.yml`.

   Use `--dry-run` first if you want to preview the changes:

   ```bash
   scripts/release-bump.sh --dry-run vX.Y.Z
   ```

   #### Manual recipe (if the script breaks)

   If `scripts/release-bump.sh` is unavailable or broken, apply the three
   edits by hand and combine them into a single commit:

   ```bash
   # Bump compose template
   sed 's/^JAMSESH_VERSION=.*/JAMSESH_VERSION=vX.Y.Z/' deploy/compose/.env.example \
     > deploy/compose/.env.example.tmp && mv deploy/compose/.env.example.tmp deploy/compose/.env.example

   # Bump plugin wrapper
   sed 's/^readonly JAMSESH_PLUGIN_VERSION=.*/readonly JAMSESH_PLUGIN_VERSION="vX.Y.Z"/' bin/jamsesh \
     > bin/jamsesh.tmp && mv bin/jamsesh.tmp bin/jamsesh

   # Bump plugin manifest (bare version, no 'v' prefix)
   jq --ascii-output --arg v "X.Y.Z" '.version = $v' .claude-plugin/plugin.json \
     > .claude-plugin/plugin.json.tmp && mv .claude-plugin/plugin.json.tmp .claude-plugin/plugin.json

   git add bin/jamsesh deploy/compose/.env.example .claude-plugin/plugin.json
   git commit -m "release-prep: vX.Y.Z"
   git tag -a vX.Y.Z -m "Release vX.Y.Z"
   git push origin main
   git push origin vX.Y.Z
   ```

3. **Confirm the CHANGELOG entry.** `release-deploy` drafts `CHANGELOG.md`
   from the bound items + their commits. Review and edit before approving.

4. **Watch the workflow.** Real-time view:

   ```bash
   gh run watch
   ```

   Or list recent runs of the release workflow:

   ```bash
   gh run list --workflow=release.yml --limit 5
   ```

5. **Verify the release.** Once `release.yml` finishes:

   ```bash
   gh release view vX.Y.Z
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
