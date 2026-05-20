#!/usr/bin/env bats
# tests/wrapper/cosign.bats — cosign verification paths for bin/jamsesh.
# bats-core v1.x required.
#
# The wrapper only runs cosign when: (a) cosign is on PATH, AND (b) the
# release server responds 200 to the .sigstore.json request.
#
# Tests:
#   1. cosign present + bundle missing: degrades to sha256-only, exits 0
#   2. cosign present + bundle "valid" (shim exits 0): exits 0
#   3. cosign absent: sha256-only path, exits 0

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
@test "cosign present + bundle missing: degrades to sha256-only, exits 0" {
  # Install a cosign shim (cosign present on PATH), but do NOT place a
  # .sigstore.json in the release dir.  The wrapper's curl for the bundle
  # will return 404 (http.server serves 404 for missing files), and the
  # wrapper should log "no sigstore bundle published" and continue.
  install_cosign_shim 0   # shim exit code doesn't matter here — bundle fetch fails first

  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]
  [[ "$output" == "test-sentinel" ]]
}

# ---------------------------------------------------------------------------
@test "cosign present + bundle invalid: dies with 'cosign verification failed'" {
  # Place a .sigstore.json in the release dir (any content — the shim ignores
  # it), but configure the cosign shim to exit 1 (verify-blob failure).
  local bundle_name="${_binary_name}.sigstore.json"
  printf '{"fake":"bundle"}\n' > "${RELEASE_DIR}/${bundle_name}"

  install_cosign_shim 1   # shim exits 1 → wrapper treats this as verify failure

  run "$(wrapper_bin)"
  [ "$status" -ne 0 ]
  [[ "$output" =~ "cosign verification failed" ]]
}

# ---------------------------------------------------------------------------
@test "cosign absent: sha256-only path succeeds (no cosign on PATH)" {
  # Do NOT install the cosign shim — the wrapper's `command -v cosign` check
  # returns nothing, so the entire verify-blob branch is skipped.
  # The SHIM_DIR is on PATH but contains no cosign binary.
  # Sanity: confirm cosign is not on our shimmed PATH.
  run command -v cosign
  [ "$status" -ne 0 ]  # cosign must not be found

  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]
  [[ "$output" == "test-sentinel" ]]
}
