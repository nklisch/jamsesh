---
id: testing-bin-jamsesh-regression-harness-bats-suite
kind: story
stage: implementing
tags: [testing, infra, plugin]
parent: testing-bin-jamsesh-regression-harness
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# bats test suite for `bin/jamsesh` (helpers + install + overrides + cosign + hygiene)

## Brief

Implement Units 1, 2, 3, and 4 from the parent feature's design:

- `tests/wrapper/helpers.bash` — shared setup/teardown and mock fixtures
- `tests/wrapper/install.bats` — cold cache, warm cache, sha256 mismatch, unsupported OS, unsupported arch (5 tests)
- `tests/wrapper/overrides.bats` — `JAMSESH_BIN_OVERRIDE` (happy + non-exec error), `JAMSESH_PLUGIN_VERSION_OVERRIDE`, `JAMSESH_PLUGIN_OWNER` (4 tests)
- `tests/wrapper/cosign.bats` — cosign present + bundle missing, cosign present + bundle invalid, cosign absent (3 tests)
- `tests/wrapper/hygiene.bats` — stdout cleanliness, tmpdir cleanup happy, tmpdir cleanup on sha256 fail (3 tests)

Total: 15 bats tests across 4 files + 1 helpers file. See the parent
feature body for the complete design with sketches and rationale.

## Approach summary

- **Framework**: bats-core v1.x.
- **Mock release endpoint**: per-test python3 -m http.server on an ephemeral port, rooted in a `BATS_TMPDIR`-derived release dir.
- **Redirect wrapper's curl calls to the mock**: PATH-shimmed `curl` that rewrites `https://github.com/...` → `http://127.0.0.1:<port>/...`. The shim exec's `/usr/bin/curl` for everything else.
- **PATH-shimmed `uname`** for the unsupported-OS/arch tests.
- **PATH-shimmed `cosign`** for the cosign tests; exit code controlled via env var consumed by the shim.
- **Cache isolation**: each test sets a unique `CLAUDE_PLUGIN_DATA` under `BATS_TMPDIR`.
- **Fake binary**: a `#!/usr/bin/env bash\nprintf 'test-sentinel\\n'\n"$@"\n` script so we can exec it and assert on stdout.

## Acceptance criteria

- [ ] `bats tests/wrapper/install.bats` exits 0 with 5 passing tests
- [ ] `bats tests/wrapper/overrides.bats` exits 0 with 4 passing tests
- [ ] `bats tests/wrapper/cosign.bats` exits 0 with 3 passing tests
- [ ] `bats tests/wrapper/hygiene.bats` exits 0 with 3 passing tests
- [ ] `bats tests/wrapper/` (whole-directory run) exits 0 with 15 passing tests
- [ ] No network egress: shimmed curl logs show only requests to `http://127.0.0.1:<port>/...`
- [ ] No orphan `${cache_dir}/.tmp.*` directories after the full run
- [ ] Tests run in <30 s on a developer laptop

## Smoke verification

After implementing, manually break `bin/jamsesh` one line at a time to
confirm the suite catches each:
- Swap the sha256 awk pattern → install.bats sha256 mismatch test fails
- Remove the `chmod +x` line → install.bats happy path fails (exec permission denied)
- Remove the EXIT trap → hygiene tmpdir test fails

Revert the breaks before committing. Document the smoke result in
implementation notes.

## Files to create

- `tests/wrapper/helpers.bash`
- `tests/wrapper/install.bats`
- `tests/wrapper/overrides.bats`
- `tests/wrapper/cosign.bats`
- `tests/wrapper/hygiene.bats`
- `tests/wrapper/README.md` (one-pager: how to run locally, bats-core version requirement)

## Files NOT modified

- `bin/jamsesh` itself — the wrapper is the SUT and must not change in this story
- `.github/workflows/quickstart.yml` — CI integration is the child story `testing-bin-jamsesh-regression-harness-ci-job`
