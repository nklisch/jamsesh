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
  local cached="${XDG_CACHE_HOME}/jamsesh/bin/jamsesh-${wrapper_version}-${_os}-${_arch}"
  [ -x "${cached}" ]
}

# ---------------------------------------------------------------------------
@test "cold install writes a .sha256 sidecar alongside the cached binary" {
  # gate-security-wrapper-cache-hit-no-resig-verify: every fresh install
  # must drop a hex sha256 sidecar so subsequent invocations can re-verify
  # cache integrity without a network round-trip.
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]

  local wrapper_version
  wrapper_version=$(grep -E '^readonly JAMSESH_PLUGIN_VERSION=' "$(wrapper_bin)" \
    | sed 's/^readonly JAMSESH_PLUGIN_VERSION="\(.*\)"/\1/')
  local cached="${XDG_CACHE_HOME}/jamsesh/bin/jamsesh-${wrapper_version}-${_os}-${_arch}"
  [ -f "${cached}.sha256" ]

  # Sidecar must be plain hex (64 chars for sha256), no whitespace.
  local sidecar_contents
  sidecar_contents=$(cat "${cached}.sha256")
  [[ "${sidecar_contents}" =~ ^[0-9a-f]{64}$ ]]
}

# ---------------------------------------------------------------------------
@test "warm cache with tampered binary: re-downloads and re-verifies" {
  # gate-security-wrapper-cache-hit-no-resig-verify: a binary whose hash
  # diverges from its sidecar must trigger a re-download (the sidecar may
  # itself be stale; either way we don't exec the tampered bytes).
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]

  local wrapper_version
  wrapper_version=$(grep -E '^readonly JAMSESH_PLUGIN_VERSION=' "$(wrapper_bin)" \
    | sed 's/^readonly JAMSESH_PLUGIN_VERSION="\(.*\)"/\1/')
  local cached="${XDG_CACHE_HOME}/jamsesh/bin/jamsesh-${wrapper_version}-${_os}-${_arch}"

  # Tamper the cached binary so its sha256 no longer matches the sidecar.
  printf 'evil-bytes' >> "${cached}"

  # Re-invoke the wrapper. The mock server is still serving the good binary,
  # so a re-download recovers automatically; the wrapper warns to stderr but
  # exits 0 after re-installing.
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]
  [[ "$output" == *"test-sentinel"* ]]
  # The cached binary is now the freshly-downloaded one — running it again
  # should hit the warm-cache verified path.
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]
  [[ "$output" == "test-sentinel" ]]
}

# ---------------------------------------------------------------------------
@test "warm cache with missing sidecar: re-downloads" {
  # gate-security-wrapper-cache-hit-no-resig-verify: legacy installs that
  # pre-date the sidecar feature have no sidecar file. Wrapper falls through
  # to the re-download path rather than exec'ing an unverified cached binary.
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]

  local wrapper_version
  wrapper_version=$(grep -E '^readonly JAMSESH_PLUGIN_VERSION=' "$(wrapper_bin)" \
    | sed 's/^readonly JAMSESH_PLUGIN_VERSION="\(.*\)"/\1/')
  local cached="${XDG_CACHE_HOME}/jamsesh/bin/jamsesh-${wrapper_version}-${_os}-${_arch}"

  # Simulate a pre-sidecar install: remove the sidecar but keep the binary.
  rm -f "${cached}.sha256"

  # Re-invoke; expect a successful exec via re-download path. The mock server
  # is still up and serves the good binary.
  run "$(wrapper_bin)"
  [ "$status" -eq 0 ]
  [[ "$output" == "test-sentinel" ]]
  # Re-installed binary has the sidecar back.
  [ -f "${cached}.sha256" ]
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
