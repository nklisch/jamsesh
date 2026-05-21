#!/usr/bin/env bash
# tests/wrapper/helpers.bash — shared fixture helpers for the bin/jamsesh bats suite.
#
# Designed to be small and composable: each helper does one thing.
# Source this file in .bats files with:
#   load helpers.bash
#
# Expected bats-core version: v1.x (tested against 1.13.0).

# ---------------------------------------------------------------------------
# setup_wrapper_test
#   Full setup for a wrapper test:
#     1. Creates an isolated CLAUDE_PLUGIN_DATA dir under BATS_TMPDIR.
#     2. Creates a per-test shim dir and prepends it to PATH.
#     3. Creates a per-test mock release dir.
#     4. Starts a Python HTTP mock release server pointing at the release dir.
#
#   Exports: CLAUDE_PLUGIN_DATA, SHIM_DIR, RELEASE_DIR, MOCK_PORT, PATH
#
#   Call this from your test file's setup() function.
# ---------------------------------------------------------------------------
setup_wrapper_test() {
  # Unique, per-test subdirectory under the bats-provided temp directory.
  # BATS_TEST_NUMBER is set by bats-core to the sequential test index.
  local test_id="${BATS_TEST_NUMBER:-$$}"

  # Isolated plugin cache dir — no test can see another test's cached binary.
  export CLAUDE_PLUGIN_DATA="${BATS_TMPDIR}/cache_${test_id}"
  mkdir -p "${CLAUDE_PLUGIN_DATA}"

  # Shim dir — prepended to PATH so our fake curl/uname/cosign shadow real ones.
  export SHIM_DIR="${BATS_TMPDIR}/shims_${test_id}"
  mkdir -p "${SHIM_DIR}"
  export PATH="${SHIM_DIR}:${PATH}"

  # Release dir — python http.server will serve files from here.
  export RELEASE_DIR="${BATS_TMPDIR}/release_${test_id}"
  mkdir -p "${RELEASE_DIR}"

  # Start the mock release server and capture the port.
  start_mock_release "${RELEASE_DIR}"
}

# ---------------------------------------------------------------------------
# teardown_wrapper_test
#   Clean up after a wrapper test:
#     1. Kills the python HTTP server (if PID was saved in MOCK_SERVER_PID).
#     2. Removes the per-test dirs we created.
#
#   Call this from your test file's teardown() function.
# ---------------------------------------------------------------------------
teardown_wrapper_test() {
  # Kill the mock HTTP server if we started one.
  if [[ -n "${MOCK_SERVER_PID:-}" ]]; then
    kill "${MOCK_SERVER_PID}" 2>/dev/null || true
    wait "${MOCK_SERVER_PID}" 2>/dev/null || true
    unset MOCK_SERVER_PID
  fi

  # Remove per-test dirs.  The cache dir is intentionally removed so leftover
  # .tmp.* orphan detection tests don't see artifacts from other tests.
  local test_id="${BATS_TEST_NUMBER:-$$}"
  rm -rf \
    "${BATS_TMPDIR}/cache_${test_id}" \
    "${BATS_TMPDIR}/shims_${test_id}" \
    "${BATS_TMPDIR}/release_${test_id}"
}

# ---------------------------------------------------------------------------
# start_mock_release <release-dir>
#   Boots `python3 -m http.server` on an ephemeral port with its root set to
#   <release-dir>.  Writes the chosen port to MOCK_PORT and the server PID
#   to MOCK_SERVER_PID (both exported).
#
#   The server URL layout mirrors the GitHub releases download layout:
#     https://github.com/<owner>/jamsesh/releases/download/<version>/<asset>
#   When the curl shim rewrites the URL it preserves the path; the release
#   dir must therefore contain files named exactly as the assets would be on
#   GitHub (e.g., "jamsesh-linux-amd64", "checksums.txt").
# ---------------------------------------------------------------------------
start_mock_release() {
  local release_dir="$1"

  # Pick an ephemeral port by binding a socket, reading the OS-assigned port,
  # then closing the socket — the python server will rebind on the same port
  # within milliseconds (race window is negligible in practice).
  local port
  port=$(python3 -c "
import socket
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('127.0.0.1', 0))
print(s.getsockname()[1])
s.close()
")
  export MOCK_PORT="${port}"

  # Start the server in the background; suppress its access log to keep bats
  # output clean.
  python3 -m http.server \
    --bind 127.0.0.1 \
    --directory "${release_dir}" \
    "${port}" \
    >/dev/null 2>&1 &
  export MOCK_SERVER_PID=$!

  # Wait for the server to be ready (up to 3 s).
  local i
  for i in $(seq 1 30); do
    if curl -s "http://127.0.0.1:${port}/" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  echo "helpers.bash: mock release server failed to start on port ${port}" >&2
  return 1
}

# ---------------------------------------------------------------------------
# write_fake_binary <release-dir> <binary-name>
#   Writes two files into <release-dir>:
#     <binary-name>     — an executable shell script that prints "test-sentinel"
#                         on stdout, then execs its arguments (so args are
#                         visible in output when the wrapper passes "$@").
#     checksums.txt     — a valid GNU sha256sum-format file containing the
#                         hash of the binary.
#
#   After this call, start_mock_release (or a previously started server) will
#   serve both files and the wrapper's sha256 check will pass.
# ---------------------------------------------------------------------------
write_fake_binary() {
  local release_dir="$1"
  local binary_name="$2"
  local binary_path="${release_dir}/${binary_name}"

  # Write the fake binary script.
  printf '#!/usr/bin/env bash\nprintf '"'"'test-sentinel\n'"'"'\n"$@"\n' \
    > "${binary_path}"
  chmod +x "${binary_path}"

  # Generate the checksum and write checksums.txt in GNU sha256sum format.
  local checksum
  checksum=$(sha256sum "${binary_path}" | awk '{print $1}')
  printf '%s  %s\n' "${checksum}" "${binary_name}" \
    > "${release_dir}/checksums.txt"
}

# ---------------------------------------------------------------------------
# install_curl_shim <base-url>
#   Writes SHIM_DIR/curl — a thin wrapper that rewrites any URL matching
#   https://github.com/<owner>/jamsesh/releases/download/<version>/...
#   to http://127.0.0.1:<MOCK_PORT>/... and then exec's /usr/bin/curl for
#   the actual fetch.  Non-github URLs are passed through unchanged.
#
#   <base-url> is the mock server's root, e.g. "http://127.0.0.1:${MOCK_PORT}"
#   The URL rewrite strips the /releases/download/<version> path prefix that
#   GitHub uses, because the mock server is rooted directly in the release dir.
#
#   Requires: MOCK_PORT to be set (done by start_mock_release).
# ---------------------------------------------------------------------------
install_curl_shim() {
  local mock_base="${1:-http://127.0.0.1:${MOCK_PORT}}"
  local shim="${SHIM_DIR}/curl"

  # The wrapper calls curl with two URL forms:
  #   https://github.com/<owner>/jamsesh/releases/download/<version>/<asset>
  # We need to rewrite just the host+prefix.  The shim rewrites positional
  # arguments inline — it cannot use sed on the argv array, so we rebuild it.
  cat > "${shim}" <<'SHIM_EOF'
#!/usr/bin/env bash
# curl shim: rewrite github.com release download URLs to the mock server.
# Actual curl binary (absolute path to avoid self-recursion).
REAL_CURL=/usr/bin/curl

MOCK_BASE="__MOCK_BASE__"

new_args=()
for arg in "$@"; do
  # Rewrite https://github.com/<owner>/<repo>/releases/download/<ver>/<asset>
  # → http://127.0.0.1:<port>/<asset>
  if [[ "${arg}" =~ ^https://github\.com/[^/]+/[^/]+/releases/download/[^/]+/(.+)$ ]]; then
    new_args+=("${MOCK_BASE}/${BASH_REMATCH[1]}")
  else
    new_args+=("${arg}")
  fi
done

exec "${REAL_CURL}" "${new_args[@]}"
SHIM_EOF

  # Substitute the placeholder with the actual mock base URL.
  sed -i "s|__MOCK_BASE__|${mock_base}|g" "${shim}"
  chmod +x "${shim}"
}

# ---------------------------------------------------------------------------
# install_uname_shim <fake-os> <fake-arch>
#   Writes SHIM_DIR/uname that returns <fake-os> for `uname -s` and
#   <fake-arch> for `uname -m`.  All other uname invocations pass through
#   to /usr/bin/uname unchanged so the shim doesn't corrupt unrelated commands.
#
#   Example: install_uname_shim "FreeBSD" "riscv64"
# ---------------------------------------------------------------------------
install_uname_shim() {
  local fake_os="$1"
  local fake_arch="$2"
  local shim="${SHIM_DIR}/uname"

  cat > "${shim}" <<SHIM_EOF
#!/usr/bin/env bash
# uname shim: mutate -s and -m output for wrapper OS/arch detection tests.
FAKE_OS="${fake_os}"
FAKE_ARCH="${fake_arch}"

if [[ "\$*" == "-s" ]]; then
  printf '%s\n' "\${FAKE_OS}"
elif [[ "\$*" == "-m" ]]; then
  printf '%s\n' "\${FAKE_ARCH}"
else
  exec /usr/bin/uname "\$@"
fi
SHIM_EOF
  chmod +x "${shim}"
}

# ---------------------------------------------------------------------------
# install_cosign_shim <exit-code>
#   Writes SHIM_DIR/cosign that exits with <exit-code>.
#   Exit 0  → cosign verify-blob succeeded (valid bundle).
#   Exit 1  → cosign verify-blob failed    (invalid bundle).
#
#   This lets tests exercise both the "cosign verification failed" hard-fail
#   path and the successful verification path without invoking the real cosign.
# ---------------------------------------------------------------------------
install_cosign_shim() {
  local exit_code="${1:-0}"
  local shim="${SHIM_DIR}/cosign"

  cat > "${shim}" <<SHIM_EOF
#!/usr/bin/env bash
# cosign shim: exits with a fixed code to simulate verify-blob outcomes.
exit ${exit_code}
SHIM_EOF
  chmod +x "${shim}"
}

# ---------------------------------------------------------------------------
# Helper: path to the wrapper script under test (SUT).
# ---------------------------------------------------------------------------
wrapper_bin() {
  # Resolve relative to this helpers file so the suite can run from any cwd.
  local helpers_dir
  helpers_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  printf '%s/../../plugins/jamsesh/bin/jamsesh\n' "${helpers_dir}"
}
