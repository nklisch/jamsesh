---
id: gate-tests-wrapper-bats-xdg-default-fallback-uncovered
kind: story
stage: review
tags: [testing, plugin, infra]
parent: null
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-25
updated: 2026-05-25
---

# Wrapper bats suite never exercises `XDG_CACHE_HOME` default fallback

## Priority
Medium

## Spec reference
Item: `story-data-dir-env-rename`
Implementation notes:
> `plugins/jamsesh/bin/jamsesh` wrapper cache path changed from
> `${CLAUDE_PLUGIN_DATA:-$HOME/.cache/jamsesh}/bin` to
> `${XDG_CACHE_HOME:-$HOME/.cache}/jamsesh/bin`.
Acceptance: clean cutover with `~/.cache` as the default.

## Gap type
Adversarial — boundary value (the unset case).

## Location
`tests/wrapper/helpers.bash:29-30` always exports `XDG_CACHE_HOME` to a
per-test temp dir; `install.bats`, `hygiene.bats`, `overrides.bats` all
run with the override set. The default-fallback path (`XDG_CACHE_HOME`
unset → `$HOME/.cache/jamsesh/bin`) has zero coverage.

## Suggested test
```bash
@test "wrapper falls back to ~/.cache when XDG_CACHE_HOME is unset" {
  unset XDG_CACHE_HOME
  HOME="$BATS_TEST_TMPDIR/home"
  mkdir -p "$HOME"
  run_jamsesh_wrapper --version
  [ -d "$HOME/.cache/jamsesh/bin" ]
}
```

## Test location (suggested)
`tests/wrapper/install.bats` or new `tests/wrapper/xdg_defaults.bats`.

## Impact
Cache path `${XDG_CACHE_HOME:-${HOME}/.cache}/jamsesh/bin` could regress
(e.g., accidentally requiring `XDG_CACHE_HOME` to be set) and no wrapper
test would catch it.

## Implementation notes

- New file `tests/wrapper/xdg_defaults.bats` covers the unset case. It does
  not call the shared `setup_wrapper_test` helper because that helper
  always exports `XDG_CACHE_HOME`. Instead it builds an equivalent fixture
  inline (per-test `HOME`, shim dir, mock release server, fake binary) and
  explicitly `unset XDG_CACHE_HOME`s before invoking the wrapper.
- Two scenarios:
  1. Positive: wrapper succeeds, prints `test-sentinel`, and the cached
     binary lands at `$HOME/.cache/jamsesh/bin/jamsesh-<ver>-<os>-<arch>`
     with a sidecar `.sha256` next to it.
  2. Negative: no cache dir materialises at `/tmp/jamsesh/bin` (a stand-in
     for stray paths the wrapper must not pick), and the documented
     `$HOME/.cache/jamsesh/bin` does exist.
- Resolves the wrapper version dynamically from the script's
  `readonly JAMSESH_PLUGIN_VERSION=` declaration so the test does not need
  to be edited on every release bump (same approach as `install.bats`).
- Verification: `bats tests/wrapper/xdg_defaults.bats` → 2/2 pass;
  `bats tests/wrapper/*.bats` → 20/20 pass (full suite unaffected).
