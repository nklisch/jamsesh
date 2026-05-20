---
id: testing-bin-jamsesh-regression-harness
kind: feature
stage: drafting
tags: [testing, infra, plugin]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-20
---

# Regression test harness for `bin/jamsesh`

## Brief

`bin/jamsesh` (129 lines of bash) is the plugin's runtime trust boundary —
on first invocation it downloads, sha256-verifies, optionally cosign-verifies,
and caches the per-arch jamsesh binary from GitHub release assets. There is
currently no regression-test harness for it. A future edit that breaks the
sha256 awk pattern, the curl flow, the cosign invocation, or the env-var
overrides will not surface until a user actually invokes the wrapper in the
field.

Build a small test suite using `bats` (or `shellspec`) covering at least:

- **Happy path**: cold cache → fetch → sha256 ok → exec. Use a local
  `httptest`-style fixture serving a known binary + matching `checksums.txt`.
- **Warm cache**: second invocation hits cache, no network.
- **sha256 mismatch**: corrupted checksums.txt → hard fail with the exact
  expected error message.
- **Unsupported OS / arch**: clear error.
- **`JAMSESH_BIN_OVERRIDE`**: skips fetch entirely, execs the override path
  with args + stdin intact.
- **`JAMSESH_PLUGIN_VERSION_OVERRIDE`**: uses the overridden version in URL +
  cache filename.
- **`JAMSESH_PLUGIN_OWNER`**: uses the overridden owner in URL + cosign
  identity-regexp.
- **Cosign present + bundle missing**: degrades to sha256-only, no error.
- **Cosign present + bundle invalid**: hard fail.
- **Stdout cleanliness**: wrapper's stdout is exactly the binary's stdout
  (critical for `.mcp.json` headersHelper).
- **tmpdir cleanup**: no `${cache_dir}/.tmp.*` orphans after any code path.

Wire the suite into CI as a new job in `.github/workflows/quickstart.yml`
(parallel to existing jobs). Job should run in <30s with no network
dependencies (mock the release endpoint).

References: parent feature `feature-cc-plugin-wrapper-binary-fetch`
(archive), wrapper file `bin/jamsesh`. The wrapper's contract is documented
in the file's header comment block.
