---
id: gate-tests-wrapper-bats-xdg-default-fallback-uncovered
kind: story
stage: implementing
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
