---
id: testing-bin-jamsesh-regression-harness
kind: feature
stage: done
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

## Design decisions

Resolved under autopilot autonomy on 2026-05-20; rationale below.

- **Framework — bats vs shellspec**: **bats** (bats-core v1.x). Rationale:
  bats is the de-facto standard for shell test suites, installable on Ubuntu
  CI via `apt-get install bats`, simpler syntax matching the size of this
  test surface (~10 cases). shellspec would be over-tooled here.
- **Test location**: `tests/wrapper/`. Adjacent to the existing `tests/e2e/`
  bucket. Name signals "tests for the wrapper" without coupling to bats-
  vs-shellspec terminology.
- **Mock release endpoint**: ephemeral-port `python3 -m http.server` rooted
  in a per-test temp dir containing the fake `jamsesh-<os>-<arch>` binary
  + `checksums.txt`. Zero external infra, deterministic, fast. Python3 is
  pre-installed on `ubuntu-latest` CI runners.
- **Cosign mock**: PATH-shimmed fake `cosign` binary written into
  `BATS_TMPDIR/bin/cosign` and prepended to `PATH`. Needed to exercise
  both "cosign present + bundle missing → degrade silently" and "cosign
  present + bundle invalid → hard fail" paths. The real cosign is not
  invoked.
- **Cache isolation**: every test sets `CLAUDE_PLUGIN_DATA` to a unique
  `BATS_TMPDIR`-derived directory in `setup()`, cleans up in `teardown()`.
  No test can see another test's cache.
- **CI integration shape**: new `wrapper-tests` job in
  `.github/workflows/quickstart.yml`, parallel to existing jobs. Installs
  bats via `apt-get install -y bats`, runs `bats tests/wrapper/`. Target
  runtime < 30s, zero network egress.
- **Version-constant tests**: deliberately skipped from this suite — the
  `JAMSESH_PLUGIN_VERSION` constant is verified against `GITHUB_REF_NAME`
  by the release workflow already (`docs/RELEASING.md`); duplicating that
  check would couple this suite to release-time concerns.

## Architectural choice

A bats-core test suite with a small mock-fixture infrastructure
(`tests/wrapper/helpers.bash`) shared across test files. Each `.bats` file
groups tests by surface (install, overrides, cosign, hygiene). The
fixtures are self-contained: a Python HTTP server stand-in for GitHub
releases, and a PATH-shimmed cosign binary for the verify-blob path.

Considered alternatives:
1. **Shellspec** — richer DSL but heavier install footprint; not justified for ~10 test cases.
2. **Go testscript harness** — would let the tests sit in `tests/e2e/`, but writing assertions about shell-script output via Go is awkward and the wrapper isn't Go code; coupling it to Go tooling adds friction without payoff.
3. **End-to-end test against a real GitHub release** — would catch real-world drift but adds 10+ seconds of network latency per test, leaks secrets to CI, and would dirty `gh.com/<owner>/jamsesh` releases with test traffic. Mock fixture is strictly better.

## Implementation Units

### Unit 1: Helpers + install/error tests

**Files**:
- `tests/wrapper/helpers.bash` (new) — shared setup/teardown + mock fixtures
- `tests/wrapper/install.bats` (new) — happy path + warm cache + sha256 mismatch + unsupported OS/arch

**Story**: `testing-bin-jamsesh-regression-harness-bats-suite` (covers Units 1, 2, 3, 4 — see Phase 7 below for why they're consolidated)

```bash
# tests/wrapper/helpers.bash sketch
#
# setup_wrapper_test     — creates BATS_TMPDIR-rooted CLAUDE_PLUGIN_DATA, PATH,
#                          fixture release dir; starts mock release server.
# teardown_wrapper_test  — kills the python server, cleans temp dirs.
# start_mock_release     — boots `python3 -m http.server` on an ephemeral port
#                          inside a temp release dir; writes the port to a
#                          shell variable callers consume.
# write_fake_binary      — writes a shell-script "binary" that prints a sentinel
#                          when exec'd, plus matching checksums.txt.
# install_cosign_shim    — writes BATS_TMPDIR/bin/cosign as a script that exits
#                          0 or 1 per env-var contract, prepends to PATH.
```

```bash
# tests/wrapper/install.bats sketch
#
# @test "cold cache: fetches binary, verifies sha256, execs cleanly"
# @test "warm cache: second invocation execs from cache, no network call"
# @test "sha256 mismatch: dies with 'sha256 mismatch' on tampered checksums.txt"
# @test "unsupported OS: dies with 'unsupported OS' for ostype that doesn't match the case branches"
# @test "unsupported arch: dies with 'unsupported arch' for archs not in {x86_64,amd64,arm64,aarch64}"
```

**Implementation Notes**:
- `mock_release_server` writes the test's chosen response into a release
  dir, then starts python's http.server with `--directory <release-dir>`.
  Read the port via a `python3 -c '...print(sock.getsockname()[1])'`
  preamble so concurrent tests don't collide.
- The wrapper's release URL is hard-coded to `https://github.com/<owner>/jamsesh/releases/download/<version>`. To redirect to the mock, **override `JAMSESH_PLUGIN_OWNER`** is insufficient (still hits github.com). The cleanest approach: PATH-shim `curl` to rewrite the host. The shim parses the URL and replaces `https://github.com/...` with `http://127.0.0.1:<port>/...`, then exec's real curl.
- The "binary" served can be a `#!/usr/bin/env bash\necho test-sentinel\n"$@"\n"` script so we can exec it and assert on stdout.
- For "unsupported OS/arch": override `uname` via PATH shim, returning the unsupported value, and assert the wrapper's die message.

**Acceptance Criteria**:
- [ ] `bats tests/wrapper/install.bats` exits 0 with 5 passing tests
- [ ] No network traffic to github.com during the run (verify via shimmed curl logs)
- [ ] No orphan `${cache_dir}/.tmp.*` directories after any test

### Unit 2: Env-var override tests

**File**: `tests/wrapper/overrides.bats` (new)

```bash
# @test "JAMSESH_BIN_OVERRIDE: skips fetch entirely, execs override path with args"
# @test "JAMSESH_BIN_OVERRIDE: not-executable override dies with clear error"
# @test "JAMSESH_PLUGIN_VERSION_OVERRIDE: uses overridden version in URL and cache filename"
# @test "JAMSESH_PLUGIN_OWNER: uses overridden owner in release URL"
```

**Implementation Notes**:
- BIN_OVERRIDE test: write a sentinel script, point `JAMSESH_BIN_OVERRIDE` at it, invoke the wrapper with args, assert stdout includes both the sentinel and the args (separately-line via the bash `"$@"` echo).
- VERSION_OVERRIDE test: assert via the shimmed curl that the URL fetched contains the overridden version and that the cached filename includes it.
- OWNER test: similarly assert curl URL.

**Acceptance Criteria**:
- [ ] `bats tests/wrapper/overrides.bats` exits 0 with 4 passing tests
- [ ] Each test isolated to its own `CLAUDE_PLUGIN_DATA` temp dir

### Unit 3: Cosign tests

**File**: `tests/wrapper/cosign.bats` (new)

```bash
# @test "cosign present + bundle missing: degrades to sha256-only, no error"
# @test "cosign present + bundle invalid: dies with 'cosign verification failed'"
# @test "cosign absent: no-op (sha256-only path); identical exit behavior to no-bundle"
```

**Implementation Notes**:
- Bundle missing: the mock release server returns 404 for `.sigstore.json` requests; the wrapper logs "no sigstore bundle published" and continues. Assert non-zero is NOT the exit.
- Bundle invalid: the mock release server serves a `.sigstore.json` file (any content), and the cosign shim returns exit 1 to simulate verification failure. Assert "cosign verification failed" in stderr.
- Cosign absent: the cosign shim is NOT installed (clean PATH excludes BATS_TMPDIR/bin), so `command -v cosign` returns nothing and the verify branch is skipped entirely.

**Acceptance Criteria**:
- [ ] `bats tests/wrapper/cosign.bats` exits 0 with 3 passing tests

### Unit 4: Hygiene tests

**File**: `tests/wrapper/hygiene.bats` (new)

```bash
# @test "stdout cleanliness: wrapper stdout is exactly the binary's stdout"
# @test "tmpdir cleanup: no orphan ${cache_dir}/.tmp.* dirs after happy path"
# @test "tmpdir cleanup: no orphan ${cache_dir}/.tmp.* dirs after sha256 failure"
```

**Implementation Notes**:
- Stdout cleanliness: the fake binary prints a known sentinel. Wrapper's stdout MUST equal exactly that — verbose logs go to stderr (per the `log()` helper in `bin/jamsesh`). This is the `.mcp.json` headersHelper contract.
- Tmpdir cleanup: post-test, `ls ${cache_dir}/.tmp.* 2>/dev/null | wc -l` must be 0. The wrapper's EXIT trap + explicit cleanup before `exec` should guarantee this.

**Acceptance Criteria**:
- [ ] `bats tests/wrapper/hygiene.bats` exits 0 with 3 passing tests

### Unit 5: CI integration

**File**: `.github/workflows/quickstart.yml` (extend)

**Story**: `testing-bin-jamsesh-regression-harness-ci-job`

Add a new top-level job:

```yaml
  wrapper-tests:
    runs-on: ubuntu-latest
    steps:
      - name: checkout
        uses: actions/checkout@v4

      - name: install bats
        run: sudo apt-get update && sudo apt-get install -y bats

      - name: run wrapper tests
        run: bats tests/wrapper/
```

**Implementation Notes**:
- Place the job at the same indent level as `quickstart:` and `compose-template:`.
- No `needs:` declaration — runs in parallel with the existing jobs.
- No timeout (bats' own runtime is the cap; expect <30s).

**Acceptance Criteria**:
- [ ] CI workflow YAML lints clean (`yamllint .github/workflows/quickstart.yml`)
- [ ] The new `wrapper-tests` job appears in GitHub Actions on a sample PR (verified post-merge or in a dry-run branch)
- [ ] Job exits 0 on a green commit, exits non-zero if any bats test fails

## Implementation Order

1. **Story `testing-bin-jamsesh-regression-harness-bats-suite`** — Units 1, 2, 3, 4 in a single agent pass. The helpers + 4 bats files form a cohesive test suite; splitting across stories would force the same agent to recreate the helpers context. No `depends_on`.
2. **Story `testing-bin-jamsesh-regression-harness-ci-job`** — Unit 5. `depends_on: [testing-bin-jamsesh-regression-harness-bats-suite]` so we wire CI only after the tests exist and pass locally.

## Testing

The feature IS test infrastructure — its own "tests" are the bats files. The acceptance signal:

- Each bats file passes locally with `bats tests/wrapper/<file>.bats`
- The full `bats tests/wrapper/` run is green
- The new CI job is green on a sample PR
- Re-running after intentionally breaking `bin/jamsesh` (e.g., breaking the sha256 awk pattern) — at least one test must fail loudly

## Risks

- **bats version skew.** Older bats (v0.x, "bats" package on some distros) lacks `@test` syntax extensions that bats-core (v1.x) provides. Mitigation: assert `bats --version` in setup or document the bats-core requirement in `tests/wrapper/README.md`.
- **Python3 http.server port collision under parallel job execution.** GitHub Actions runs each job in its own VM, so cross-job collision is impossible. Cross-test within a single bats run uses ephemeral ports per test. Risk: low.
- **PATH-shimmed `curl` breaks if the system curl has a non-default location** (e.g., `/usr/local/bin/curl` shadowing). Mitigation: the shim invokes `/usr/bin/curl` explicitly via absolute path.
- **PATH-shimmed `uname` shim leaks into other commands** (`uname -r` etc. during the test). Mitigation: the shim only mutates output for `-s` and `-m` flags, passes through everything else.
- **Cosign shim ABI drift.** If the real cosign changes its CLI surface, our shim might not match. Mitigation: the wrapper invokes a fixed set of cosign flags (`verify-blob --bundle --certificate-identity-regexp --certificate-oidc-issuer`); the shim only needs to recognize those.
- **The wrapper's hard-coded `https://github.com/...` URL means our shim has to intercept curl, not redirect via DNS or env var.** Documented in Unit 1.

## Children complete (2026-05-20)

Both child stories advanced to `done`:

- `testing-bin-jamsesh-regression-harness-bats-suite` — 15 bats tests, 3.0s runtime, smoke-verified (`81e46ad` … `eaa8818`)
- `testing-bin-jamsesh-regression-harness-ci-job` — `wrapper-tests` job added to `quickstart.yml`, runs in parallel (`d1ddba5`)

Net deliverables: 6 new files under `tests/wrapper/` (helpers + 4 .bats files + README), 12 lines added to `.github/workflows/quickstart.yml`. Local bats smoke green; first GitHub Actions run will be the final integration check.

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: The bats apt-availability check (CI step `sudo apt-get install -y bats`) is the only unknown that local testing can't cover. Noted in the CI-job story's review section; not worth blocking on.

**Notes**: Feature delivers exactly the brief — 10 test cases the brief enumerated + 5 additions (cosign-absent, BIN_OVERRIDE non-exec error, two extra tmpdir-cleanup variants) that close gaps the brief implied. Per-child reviews approved cleanly with no blockers. Foundation docs not affected. No public-API breakage (test-only addition). The smoke verification — intentionally breaking the wrapper in 3 ways and confirming the right tests fail — is the signal that this suite isn't vacuous; it's the test-integrity contract this feature exists to enforce.
