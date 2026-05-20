---
id: testing-bin-jamsesh-regression-harness-bats-suite
kind: story
stage: review
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

- [x] `bats tests/wrapper/install.bats` exits 0 with 5 passing tests
- [x] `bats tests/wrapper/overrides.bats` exits 0 with 4 passing tests
- [x] `bats tests/wrapper/cosign.bats` exits 0 with 3 passing tests
- [x] `bats tests/wrapper/hygiene.bats` exits 0 with 3 passing tests
- [x] `bats tests/wrapper/` (whole-directory run) exits 0 with 15 passing tests
- [x] No network egress: shimmed curl logs show only requests to `http://127.0.0.1:<port>/...`
- [x] No orphan `${cache_dir}/.tmp.*` directories after the full run
- [x] Tests run in <30 s on a developer laptop

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

## Implementation notes

### bats version

Developed and verified against **bats-core 1.13.0** (installed via npm on Fedora/Linux).

### Total runtime

```
real    0m2.976s
user    0m1.393s
sys     0m0.691s
```

Well under the 30 s target.

### Smoke verification results

All three intentional breaks were verified to cause the expected test failures:

1. **Swap awk `print $1` → `print $2`** (extracts filename instead of hash):
   - Fails: install.bats cold cache (test 7), warm cache (test 8), and all
     other cold-cache-dependent tests (cosign, hygiene, overrides).
   - Note: the sha256-mismatch test (test 9) *passes* through this break
     because it tampers checksums.txt directly and `print $2` still
     returns a non-matching value; the test assertion `"sha256 mismatch"`
     still fires.

2. **Remove `chmod +x` line**:
   - Fails: install.bats cold cache (test 7), warm cache (test 8), and all
     tests that depend on a successful cold-cache install.

3. **Remove `trap 'rm -rf "${tmpdir}"' EXIT`**:
   - Fails: hygiene.bats test 6 (`tmpdir cleanup: no .tmp.* orphans after
     sha256 verification failure`) — exactly the targeted test.
   - All other tests continue to pass because the happy-path wrapper
     explicitly calls `rm -rf "${tmpdir}"; trap - EXIT` before exec.

### Deviations from the design

1. **`write_fake_binary` uses `printf` for script generation** — the
   `printf '...\n...'` idiom expands `\n` to literal newlines which is
   valid for the fake binary (the newlines are the intended line separators).
   However, for override scripts in `overrides.bats` this caused a subtle
   bug: `printf '...%s\n' "$*"` split across a literal newline meant `%s`
   was on a separate line from `"$*"`, losing the format argument. Fixed by
   using heredocs for override script generation in `overrides.bats`.

2. **`wrapper_bin()` helper** — The planned `BASH_SOURCE[0]` approach works
   correctly because bats `source`s `helpers.bash` at its real filesystem
   path (not from the bats temp dir). The helper resolves to
   `/home/nathan/dev/jamsesh/bin/jamsesh` as expected.

3. **Smoke break 1 differs slightly from the story spec** — The story
   expected "swap the sha256 awk pattern → install.bats sha256 mismatch
   test fails". In practice, swapping `print $1` → `print $2` makes the
   *cold cache* test fail (sha256 mismatch fires during install), while
   the dedicated sha256-mismatch test still passes because it tampers the
   checksum independently. Both behaviors correctly signal that the sha256
   verification path is broken.

### Additions beyond the planned set

No fixtures or helpers beyond the planned set were added. The `wrapper_bin()`
helper was added to `helpers.bash` (it was in the workflow instructions but
not explicitly in the feature's helpers sketch) — it resolves the absolute
path to `bin/jamsesh` so test files don't hardcode the repo root.
