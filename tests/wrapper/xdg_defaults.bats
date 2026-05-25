#!/usr/bin/env bats
# tests/wrapper/xdg_defaults.bats — covers the wrapper's documented default
# cache location when XDG_CACHE_HOME is unset.
#
# The other wrapper bats suites (install.bats, hygiene.bats, overrides.bats)
# always export XDG_CACHE_HOME via `setup_wrapper_test`, leaving the
# `${XDG_CACHE_HOME:-${HOME}/.cache}` fallback in `plugins/jamsesh/bin/jamsesh`
# untested. A regression there — e.g. accidentally making XDG_CACHE_HOME
# mandatory — would silently break every user with a default-XDG setup
# without any existing test catching it.
#
# This file builds its own minimal fixture (no `setup_wrapper_test`) so it
# can drop XDG_CACHE_HOME entirely while still keeping HOME pointed at a
# per-test temp dir.
#
# Story: gate-tests-wrapper-bats-xdg-default-fallback-uncovered
# Spec: story-data-dir-env-rename

load helpers.bash

setup() {
  # Per-test id so parallel bats invocations don't collide.
  local test_id="${BATS_TEST_NUMBER:-$$}"

  # Per-test isolated HOME — the wrapper must land its cache under
  # ${HOME}/.cache/jamsesh/bin when XDG_CACHE_HOME is unset.
  export HOME="${BATS_TMPDIR}/xdg_default_home_${test_id}"
  mkdir -p "${HOME}"

  # Shim dir for curl + (optionally) cosign, prepended to PATH.
  export SHIM_DIR="${BATS_TMPDIR}/xdg_default_shims_${test_id}"
  mkdir -p "${SHIM_DIR}"
  export PATH="${SHIM_DIR}:${PATH}"

  # Mock release dir + server (reuses helpers.bash).
  export RELEASE_DIR="${BATS_TMPDIR}/xdg_default_release_${test_id}"
  mkdir -p "${RELEASE_DIR}"
  start_mock_release "${RELEASE_DIR}"

  # Detect host OS/arch so we write the right asset name.
  _real_uname_s=$(uname -s)
  _real_uname_m=$(uname -m)
  case "${_real_uname_s}" in
    Linux)  _os=linux  ;;
    Darwin) _os=darwin ;;
    *)      _os=linux  ;;
  esac
  case "${_real_uname_m}" in
    x86_64|amd64)   _arch=amd64 ;;
    arm64|aarch64)  _arch=arm64 ;;
    *)              _arch=amd64 ;;
  esac
  _binary_name="jamsesh-${_os}-${_arch}"
  write_fake_binary "${RELEASE_DIR}" "${_binary_name}"
  install_curl_shim "http://127.0.0.1:${MOCK_PORT}"

  # CRITICAL: ensure XDG_CACHE_HOME is unset so the wrapper takes the
  # ${HOME}/.cache fallback branch.
  unset XDG_CACHE_HOME
}

teardown() {
  if [[ -n "${MOCK_SERVER_PID:-}" ]]; then
    kill "${MOCK_SERVER_PID}" 2>/dev/null || true
    wait "${MOCK_SERVER_PID}" 2>/dev/null || true
    unset MOCK_SERVER_PID
  fi
  local test_id="${BATS_TEST_NUMBER:-$$}"
  rm -rf \
    "${BATS_TMPDIR}/xdg_default_home_${test_id}" \
    "${BATS_TMPDIR}/xdg_default_shims_${test_id}" \
    "${BATS_TMPDIR}/xdg_default_release_${test_id}"
}

# ---------------------------------------------------------------------------
@test "wrapper falls back to \$HOME/.cache/jamsesh/bin when XDG_CACHE_HOME is unset" {
  # Sanity: confirm the env really is missing — otherwise the test passes for
  # the wrong reason.
  [ -z "${XDG_CACHE_HOME:-}" ]

  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]
  [[ "$output" == "test-sentinel" ]]

  # Resolve the version the wrapper pinned at install time, so the assertion
  # tracks future release-bumps without test edits.
  local wrapper_version
  wrapper_version=$(grep -E '^readonly JAMSESH_PLUGIN_VERSION=' "$(wrapper_bin)" \
    | sed 's/^readonly JAMSESH_PLUGIN_VERSION="\(.*\)"/\1/')

  # The cache must materialise under the documented default path:
  #   $HOME/.cache/jamsesh/bin/jamsesh-<version>-<os>-<arch>
  local cached="${HOME}/.cache/jamsesh/bin/jamsesh-${wrapper_version}-${_os}-${_arch}"
  [ -x "${cached}" ]
  [ -f "${cached}.sha256" ]
}

# ---------------------------------------------------------------------------
@test "wrapper does NOT create a cache outside \$HOME/.cache when XDG_CACHE_HOME is unset" {
  # Negative assertion: the fallback must use $HOME/.cache, never a stray
  # path like /tmp/jamsesh/bin or the cwd.
  [ -z "${XDG_CACHE_HOME:-}" ]

  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]

  # No jamsesh cache dir under /tmp (we use BATS_TMPDIR for fixtures, so
  # check the specific path the wrapper would mistakenly use).
  [ ! -d "/tmp/jamsesh/bin" ]

  # Cache MUST exist under $HOME/.cache.
  [ -d "${HOME}/.cache/jamsesh/bin" ]
}
