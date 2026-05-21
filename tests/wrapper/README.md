# `tests/wrapper/` — bats regression suite for `plugins/jamsesh/bin/jamsesh`

Pins the contract of the `plugins/jamsesh/bin/jamsesh` wrapper script:
download, sha256 verification, optional cosign verification, caching, and
env-var overrides. The bats files still refer to "the wrapper" or
"`bin/jamsesh`" in their own commentary because the binary's relative
location inside the installed plugin remains `bin/jamsesh` — only the
checked-in source path changed.

## Requirements

- **bats-core v1.x** (`bats --version` must print `Bats 1.x.x`). On Ubuntu CI:
  `sudo apt-get install -y bats`. Via npm: `npm install -g bats`.
- **python3** (stdlib `http.server`) — for the mock release endpoint.
- **sha256sum** — for checksum generation in fixtures.
- **curl** at `/usr/bin/curl` — the curl shim exec's this absolute path.

## Running locally

```
bats tests/wrapper/
```

Run a single file:

```
bats tests/wrapper/install.bats
```

Run with verbose output on failure:

```
bats --verbose-run tests/wrapper/
```

Expected runtime: < 5 s on a developer laptop.

## File layout

```
tests/wrapper/
  helpers.bash      — shared setup/teardown + fixture helpers
  install.bats      — cold cache, warm cache, sha256 mismatch, unsupported OS/arch  (5 tests)
  overrides.bats    — JAMSESH_BIN_OVERRIDE, VERSION_OVERRIDE, OWNER override        (4 tests)
  cosign.bats       — cosign present/absent, bundle missing/invalid                  (3 tests)
  hygiene.bats      — stdout cleanliness, tmpdir orphan cleanup                      (3 tests)
```

## Fixture approach

Each test gets an isolated environment:

- **`CLAUDE_PLUGIN_DATA`** is set to a unique `BATS_TMPDIR`-derived directory
  per test so no cache leaks between tests.
- **Mock release server** — a `python3 -m http.server` on an ephemeral port
  serves a fake binary + `checksums.txt` from a per-test temp directory.
  Zero network egress: no requests reach `github.com`.
- **PATH-shimmed `curl`** — `SHIM_DIR/curl` rewrites
  `https://github.com/<owner>/jamsesh/releases/download/<version>/<asset>`
  to `http://127.0.0.1:<port>/<asset>` and exec's `/usr/bin/curl` for the
  actual fetch. Non-github URLs pass through unchanged.
- **PATH-shimmed `uname`** — for unsupported-OS/arch tests; returns configured
  fake values for `-s` and `-m`; passes everything else to `/usr/bin/uname`.
- **PATH-shimmed `cosign`** — for cosign tests; exits with a configured code
  to simulate verify-blob outcomes without invoking the real cosign.

## Smoke verification

After any change to `plugins/jamsesh/bin/jamsesh`, run the suite and verify it catches regressions:

| Intentional break                         | Tests that fail                                    |
|-------------------------------------------|----------------------------------------------------|
| Swap awk `print $1` → `print $2`          | install.bats: cold cache, warm cache (sha256 mismatch triggers) |
| Remove `chmod +x` line                    | install.bats: cold cache, warm cache; most others  |
| Remove `trap ... EXIT`                    | hygiene.bats: tmpdir cleanup after sha256 failure  |
