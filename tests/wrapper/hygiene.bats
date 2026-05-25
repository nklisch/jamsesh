#!/usr/bin/env bats
# tests/wrapper/hygiene.bats — stdout cleanliness and tmpdir cleanup.
# bats-core v1.x required.
#
# Tests:
#   1. stdout cleanliness: wrapper stdout is exactly the binary's stdout
#   2. tmpdir cleanup (happy path): no ${cache_dir}/bin/.tmp.* orphans
#   3. tmpdir cleanup (sha256 failure): no ${cache_dir}/bin/.tmp.* orphans

load helpers.bash

setup() {
  setup_wrapper_test
  # Detect current OS/arch for naming release assets.
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
}

teardown() {
  teardown_wrapper_test
}

# ---------------------------------------------------------------------------
@test "stdout cleanliness: wrapper stdout is exactly the binary's stdout" {
  # The fake binary prints exactly "test-sentinel" on stdout.
  # The wrapper's log() helper writes to stderr; nothing else goes to stdout.
  # This matters: Claude Code reads the wrapper's stdout as JSON for MCP.
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]
  # Exact match — no leading/trailing whitespace, no extra lines.
  [ "$output" = "test-sentinel" ]
}

# ---------------------------------------------------------------------------
@test "tmpdir cleanup: no .tmp.* orphans after happy-path cold-cache fetch" {
  # Run a cold-cache invocation (no binary in cache yet).
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]

  # The wrapper removes the tmpdir before exec (EXIT trap removed before exec,
  # explicit rm -rf before exec).  No .tmp.* dirs should remain.
  local cache_bin_dir="${XDG_CACHE_HOME}/jamsesh/bin"
  local orphan_count
  orphan_count=$(find "${cache_bin_dir}" -maxdepth 1 -name '.tmp.*' 2>/dev/null | wc -l)
  [ "${orphan_count}" -eq 0 ]
}

# ---------------------------------------------------------------------------
@test "tmpdir cleanup: no .tmp.* orphans after sha256 verification failure" {
  # Tamper with checksums.txt to trigger a sha256 mismatch (non-zero exit).
  printf '%s  %s\n' \
    "0000000000000000000000000000000000000000000000000000000000000000" \
    "${_binary_name}" \
    > "${RELEASE_DIR}/checksums.txt"

  run "$(wrapper_bin)"
  [ "$status" -ne 0 ]  # must fail

  # Even on failure the EXIT trap fires and cleans up the tmpdir.
  local cache_bin_dir="${XDG_CACHE_HOME}/jamsesh/bin"
  local orphan_count
  orphan_count=$(find "${cache_bin_dir}" -maxdepth 1 -name '.tmp.*' 2>/dev/null | wc -l)
  [ "${orphan_count}" -eq 0 ]
}
