---
id: feature-cc-plugin-wrapper-binary-fetch-release-workflow
kind: story
stage: review
tags: [infra, plugin]
parent: feature-cc-plugin-wrapper-binary-fetch
depends_on: [feature-cc-plugin-wrapper-binary-fetch-script]
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Release workflow — delete marketplace job, add version assertion

## Scope

Edit `.github/workflows/release.yml`:

1. **Delete the `marketplace:` job** (currently lines ~265–331). The job
   pushed plugin source + per-arch binaries to `nklisch/jamsesh-cc-plugin`.
   That mirror repo no longer exists; the wrapper script fetches binaries
   directly from GH release assets instead.

2. **Add a version-assertion step** to the `sign-and-release` job that
   confirms `bin/jamsesh` has `JAMSESH_PLUGIN_VERSION` matching the
   pushed tag. Place AFTER `actions/checkout@v4`, BEFORE
   `actions/download-artifact`. Step shape from the parent feature's Unit 2.

## Implementation

Use the exact step YAML from the parent feature's Unit 2 spec:

```yaml
- name: assert bin/jamsesh version matches tag
  run: |
    set -euo pipefail
    expected="${GITHUB_REF_NAME}"
    actual=$(grep '^readonly JAMSESH_PLUGIN_VERSION=' bin/jamsesh \
             | sed -E 's/.*="([^"]+)".*/\1/')
    if [[ "${actual}" != "${expected}" ]]; then
      echo "::error::bin/jamsesh JAMSESH_PLUGIN_VERSION='${actual}' but tag is '${expected}'."
      echo "::error::Bump the constant in bin/jamsesh before pushing the tag (see docs/RELEASING.md)."
      exit 1
    fi
```

After the edit, the `jobs:` map has exactly: `build`, `sign-and-release`,
`docker`. No `marketplace`.

## Acceptance Criteria

- [ ] `marketplace:` job removed entirely from `.github/workflows/release.yml`.
- [ ] `sign-and-release` job has the new `assert bin/jamsesh version matches tag`
      step positioned right after the checkout step.
- [ ] No reference to `MARKETPLACE_DEPLOY_KEY`, `vars.MARKETPLACE_OWNER`,
      or `jamsesh-cc-plugin` anywhere in the workflow file.
- [ ] `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"`
      parses cleanly (or equivalent yq check).
- [ ] Manual simulation of the assertion locally passes when `bin/jamsesh`'s
      constant matches a target tag, fails when it doesn't.

## Notes

- The build matrix (`build:` job) is unchanged. It still produces 10
  binaries (`portal` + `jamsesh` × 5 OS/arch). The `sign-and-release` job
  uploads them all to the GH release where the wrapper fetches them.
- Don't delete the cosign signing of the `jamsesh-*` binaries — the
  wrapper's optional cosign verify-blob path depends on the bundles
  being there.
- After this story merges, the next `v*` tag push will run a clean
  workflow with no marketplace step. The currently-failing-on-marketplace
  prior v0.1.0 run can be ignored (the failed job is for a tree that no
  longer matters).

## Implementation Notes

### Edit 1 — Delete `marketplace:` job

Deleted lines 264–332 of the original file (the blank line separating
`docker:` from `marketplace:` through the final `git push origin "${VERSION}"`
line). The `jobs:` map now contains exactly `build`, `sign-and-release`,
`docker`.

### Edit 2 — Insert version-assertion step

Inserted 12-line step block immediately after `uses: actions/checkout@v4`
in `sign-and-release` (before the `actions/download-artifact@v4` step
that downloads dist-staging). Indentation matches the existing 6-space
style used by all sibling steps.

### Verification outcomes

1. YAML parse: `python3 -c "import yaml; yaml.safe_load(...)"` — exit 0,
   no output. **PASS**

2. Jobs structure: `sorted(w['jobs'].keys())` →
   `['build', 'docker', 'sign-and-release']` — no `marketplace`. **PASS**

3. Marketplace references: `grep -n "marketplace|MARKETPLACE|jamsesh-cc-plugin"` →
   no output. **PASS**

4. Assertion match simulation (`expected="v0.1.0"`): prints `OK`. **PASS**

5. Assertion mismatch simulation (`expected="v9.9.9"`): prints `MISMATCH`.
   **PASS**
