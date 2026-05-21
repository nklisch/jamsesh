#!/usr/bin/env bats
# tests/wrapper/install.bats — cold/warm cache, sha256 mismatch, unsupported OS/arch.
# bats-core v1.x required.

load helpers.bash

setup() {
  setup_wrapper_test
  # Detect the current OS/arch so we name mock assets correctly.
  _real_uname_s=$(uname -s)
  _real_uname_m=$(uname -m)
  case "${_real_uname_s}" in
    Linux)  _os=linux  ;;
    Darwin) _os=darwin ;;
    *)      _os=linux  ;;  # fallback for test host
  esac
  case "${_real_uname_m}" in
    x86_64|amd64)   _arch=amd64 ;;
    arm64|aarch64)  _arch=arm64 ;;
    *)              _arch=amd64 ;;
  esac
  _binary_name="jamsesh-${_os}-${_arch}"

  # Write the fake binary and checksums into the release dir.
  write_fake_binary "${RELEASE_DIR}" "${_binary_name}"

  # Install the curl shim so github.com URLs are redirected to the mock server.
  install_curl_shim "http://127.0.0.1:${MOCK_PORT}"
}

teardown() {
  teardown_wrapper_test
}

# ---------------------------------------------------------------------------
@test "cold cache: fetches binary, verifies sha256, execs cleanly" {
  # No binary in cache yet — wrapper must fetch, verify, install, then exec.
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]
  # The fake binary prints this exact string on stdout.
  [[ "$output" == "test-sentinel" ]]
  # After exec the cached binary must exist. Read the wrapper's pinned
  # JAMSESH_PLUGIN_VERSION at test time so the assertion tracks release-bumps
  # (hardcoding the version here makes the test drift after every release).
  local wrapper_version
  wrapper_version=$(grep -E '^readonly JAMSESH_PLUGIN_VERSION=' "$(wrapper_bin)" \
    | sed 's/^readonly JAMSESH_PLUGIN_VERSION="\(.*\)"/\1/')
  local cached="${CLAUDE_PLUGIN_DATA}/bin/jamsesh-${wrapper_version}-${_os}-${_arch}"
  [ -x "${cached}" ]
}

# ---------------------------------------------------------------------------
@test "warm cache: second invocation execs from cache without network" {
  # Prime the cache with the first invocation.
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]

  # Kill the mock server so any network attempt would fail.
  kill "${MOCK_SERVER_PID}" 2>/dev/null || true
  wait "${MOCK_SERVER_PID}" 2>/dev/null || true
  unset MOCK_SERVER_PID

  # Second invocation must succeed from cache alone.
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]
  [[ "$output" == "test-sentinel" ]]
}

# ---------------------------------------------------------------------------
@test "sha256 mismatch: dies with 'sha256 mismatch' message" {
  # Tamper: overwrite checksums.txt with a bogus hash.
  printf '%s  %s\n' "0000000000000000000000000000000000000000000000000000000000000000" \
    "${_binary_name}" > "${RELEASE_DIR}/checksums.txt"

  run "$(wrapper_bin)"
  [ "$status" -ne 0 ]
  [[ "$output" =~ "sha256 mismatch" ]]
}

# ---------------------------------------------------------------------------
@test "unsupported OS: dies with 'unsupported OS' message" {
  # Install a uname shim that reports a fictitious OS.
  install_uname_shim "FreeBSD" "x86_64"

  run "$(wrapper_bin)"
  [ "$status" -ne 0 ]
  [[ "$output" =~ "unsupported OS" ]]
}

# ---------------------------------------------------------------------------
@test "unsupported arch: dies with 'unsupported arch' message" {
  # Install a uname shim that returns a real OS but an unsupported arch.
  install_uname_shim "${_real_uname_s}" "riscv64"

  run "$(wrapper_bin)"
  [ "$status" -ne 0 ]
  [[ "$output" =~ "unsupported arch" ]]
}
